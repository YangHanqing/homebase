package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yanghanqing/homebase/internal/devices"
)

const (
	launchdLabel  = "com.yanghanqing.homebase"
	systemdUnit   = "homebase.service"
	readyTimeout  = 3 * time.Second
	probeTimeout  = time.Second
	probeInterval = 100 * time.Millisecond
)

type probeKind int

const (
	probeNotRunning probeKind = iota
	probeOurs
	probeOther
)

type probe struct {
	kind   probeKind
	listen string
}

func runStart(args []string) int {
	fs := flag.NewFlagSet("homebase start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.json")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	explicitConfig := flagWasSet(fs, "config")

	if err := requireTmux(); err != nil {
		return 2
	}
	if err := checkUserBus(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	r, err := setup(*configPath, "", 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	p := probeLoopback(r.res.Port)
	if p.kind == probeOther {
		fmt.Fprintf(os.Stderr, "port %d is in use by something else\n", r.res.Port)
		return 1
	}

	exe, err := executablePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "executable: %v\n", err)
		return 1
	}
	serveConfig := ""
	if explicitConfig {
		serveConfig = r.store.Path()
	}

	if err := applyService(exe, serveConfig); err != nil {
		fmt.Fprintf(os.Stderr, "service: %v\n", err)
		return 1
	}

	fallback := serviceProbeFallback(r)
	if !waitReady(r.res.Port, fallback) {
		if recoverAfterFailedStart(exe) {
			if err := applyService(exe, serveConfig); err == nil && waitReady(r.res.Port, fallback) {
				warnSessionLinger()
				return printStartSuccess(r)
			}
		}
		fmt.Fprintln(os.Stderr, "Homebase did not become ready.")
		fmt.Fprintln(os.Stderr, logHint())
		return 1
	}
	warnSessionLinger()
	return printStartSuccess(r)
}

func runStop(args []string) int {
	fs := flag.NewFlagSet("homebase stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if err := stopService(); err != nil {
		fmt.Fprintf(os.Stderr, "stop: %v\n", err)
		return 1
	}
	return 0
}

func runRestart(args []string) int {
	if code := runStop(nil); code != 0 {
		return code
	}
	return runStart(args)
}

func executablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func probeLoopback(port int) probe {
	return probeHealth(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
}

func probeHealth(rawURL string) probe {
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		if isUnreachable(err) {
			return probe{kind: probeNotRunning}
		}
		return probe{kind: probeOther}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return probe{kind: probeOther}
	}
	var body struct {
		OK     bool   `json:"ok"`
		Listen string `json:"listen"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || !body.OK {
		return probe{kind: probeOther}
	}
	return probe{kind: probeOurs, listen: body.Listen}
}

func isUnreachable(err error) bool {
	var ne *net.OpError
	if errors.As(err, &ne) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no connection") ||
		strings.Contains(msg, "connect: ")
}

func serviceProbeFallback(r *resolved) bool {
	// 0.0.0.0 covers loopback; a concrete Tailscale IP does not, so health
	// on 127.0.0.1 will miss a live process and we ask the service manager.
	return !r.res.Loopback && !r.res.Unspecified
}

func waitReady(port int, allowServiceFallback bool) bool {
	deadline := time.Now().Add(readyTimeout)
	for {
		p := probeLoopback(port)
		if p.kind == probeOurs {
			return true
		}
		if time.Now().After(deadline) {
			return allowServiceFallback && serviceActive()
		}
		time.Sleep(probeInterval)
	}
}

func printStartSuccess(r *resolved) int {
	fmt.Println("Homebase is running.")
	if r.res.Loopback {
		fmt.Printf("Open  %s     (this machine only)\n", loopbackURL(r.res.Port))
		fmt.Println()
		fmt.Println("To reach this from another device:")
		fmt.Printf("  1. open %s/settings.html on this machine\n", loopbackURL(r.res.Port))
		fmt.Println("  2. set Access to Trusted range (or LAN, if you understand the risk)")
		fmt.Println("  3. homebase restart")
		fmt.Println("  4. homebase pair")
		return 0
	}

	fmt.Printf("Open  %s\n", strings.TrimRight(baseURL(r.res), "/"))
	if !r.res.NeedsAuth() {
		return 0
	}
	list, err := r.dev.List()
	if err != nil || len(list) > 0 {
		return 0
	}
	token, expires, err := r.dev.Mint(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not mint a pairing link: %v\nrun 'homebase pair'\n", err)
		return 0
	}
	fmt.Println()
	fmt.Println("No devices paired yet — open this link on the device you want to authorize:")
	printPairLinks(pairURLs(r.res, token), expires.Format("15:04:05"), devices.TokenTTL.Minutes())
	fmt.Println()
	return 0
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func renderLaunchdPlist(exe, logFile, configPath string) string {
	args := "      <string>" + xmlEscape(exe) + "</string>\n      <string>serve</string>"
	if configPath != "" {
		args += "\n      <string>-config</string>\n      <string>" + xmlEscape(configPath) + "</string>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + xmlEscape(launchdLabel) + `</string>
  <key>ProgramArguments</key>
  <array>
` + args + `
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>` + xmlEscape(logFile) + `</string>
  <key>StandardErrorPath</key>
  <string>` + xmlEscape(logFile) + `</string>
</dict>
</plist>
`
}

func quoteSystemd(s string) string {
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func renderSystemdUnit(exe, configPath string) string {
	execStart := quoteSystemd(exe) + " serve"
	if configPath != "" {
		execStart += " -config " + quoteSystemd(configPath)
	}
	return `[Unit]
Description=Homebase
After=network.target

[Service]
Type=simple
ExecStart=` + execStart + `
Restart=on-failure

[Install]
WantedBy=default.target
`
}
