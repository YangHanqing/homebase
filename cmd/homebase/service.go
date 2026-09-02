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
	kind    probeKind
	listen  string
	started time.Time
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

	port, ours, err := pickListenPort(r.res.Port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ours && port != r.res.Port {
		if err := r.store.SetListenPort(port); err != nil {
			fmt.Fprintf(os.Stderr, "port: %v\n", err)
			return 1
		}
		r, err = setup(*configPath, "", 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
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
		OK      bool   `json:"ok"`
		Listen  string `json:"listen"`
		Started string `json:"started"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || !body.OK {
		return probe{kind: probeOther}
	}
	p := probe{kind: probeOurs, listen: body.Listen}
	if body.Started != "" {
		if t, err := time.Parse(time.RFC3339, body.Started); err == nil {
			p.started = t
		}
	}
	return p
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

const maxPortShift = 10

func pickListenPort(start int) (port int, ours bool, err error) {
	port = start
	if port <= 0 {
		port = 1990
	}
	for i := 0; i < maxPortShift; i++ {
		switch probeLoopback(port).kind {
		case probeOurs:
			return port, true, nil
		case probeNotRunning:
			return port, false, nil
		}
		port++
	}
	return 0, false, fmt.Errorf("ports %d–%d are in use by something else", start, start+maxPortShift-1)
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
