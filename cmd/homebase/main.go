package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/yanghanqing/homebase/internal/auth"
	"github.com/yanghanqing/homebase/internal/config"
	"github.com/yanghanqing/homebase/internal/devices"
	"github.com/yanghanqing/homebase/internal/listen"
)

func main() {
	if len(os.Args) < 2 {
		printHelp(os.Stdout)
		os.Exit(0)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "help", "-h", "--help":
		printHelp(os.Stdout)
		os.Exit(0)
	case "start":
		os.Exit(runStart(args))
	case "stop":
		os.Exit(runStop(args))
	case "restart":
		os.Exit(runRestart(args))
	case "status":
		os.Exit(runStatus(args))
	case "pair":
		os.Exit(runPair(args))
	case "version", "-v", "--version":
		os.Exit(runVersion(args))
	case "serve":
		os.Exit(runServe(args))
	case "devices":
		// Hidden: Settings is the public surface. Kept so a broken UI is
		// not the only way to revoke a device.
		os.Exit(runDevices(args))
	case "access":
		// Hidden: Settings → Access is the public surface.
		os.Exit(runAccess(args))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printHelp(os.Stderr)
		os.Exit(2)
	}
}

const helpText = `Homebase is a self-hosted web terminal. It attaches a browser to a tmux
session on this machine.

Usage:
  homebase <command>

Commands:
  start      install as a user service and run in the background
  stop       stop the service (config and tmux are left alone)
  restart    stop, then start
  status     service, bind address, URL, tmux, version
  pair       print a one-time login URL for another device
  version    print the build version

After 'homebase start', open the printed URL on this machine. To reach it
from another device: Settings → Access, then 'homebase pair'.
`

func printHelp(w io.Writer) {
	fmt.Fprint(w, helpText)
}

// resolved bundles everything the subcommands need to agree on where Homebase
// listens. They all recompute it the same way rather than asking the running
// server, so pairing works even while the server is down.
type resolved struct {
	store *config.Store
	cfg   config.File
	res   listen.Result
	dev   *devices.Store
}

func setup(configPath, listenFlag string, portFlag int) (*resolved, error) {
	path := configPath
	if path == "" {
		path = config.DefaultPath()
	}
	store, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg := store.Snapshot()

	port := cfg.ListenPort
	if portFlag != 0 {
		port = portFlag
	}
	if port == 0 {
		port = config.DefaultPort
	}
	res, err := listen.Resolve(cfg.Access, listenFlag, cfg.ListenAddr, port, cfg.TrustedRanges, listen.TailscaleIPv4)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	dev, err := devices.Open(devices.DefaultPath(path))
	if err != nil {
		return nil, fmt.Errorf("devices: %w", err)
	}
	return &resolved{store: store, cfg: cfg, res: res, dev: dev}, nil
}

// parseFlags returns an exit code. cont is false on -h / parse error: the
// caller should return code immediately and not run the command.
func parseFlags(fs *flag.FlagSet, args []string) (code int, cont bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	return 0, true
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func offLoopbackURLs(res listen.Result) []string {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			return
		}
		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			return
		}
		if h == "0.0.0.0" || h == "::" {
			return
		}
		seen[h] = true
		if strings.Contains(h, ":") {
			h = "[" + h + "]"
		}
		out = append(out, fmt.Sprintf("http://%s:%d", h, res.Port))
	}
	add(res.Host)
	for _, a := range res.BindAddrs() {
		h, _, err := net.SplitHostPort(a)
		if err != nil {
			add(a)
			continue
		}
		add(h)
	}
	return out
}

func baseURL(res listen.Result) string {
	host := res.Host
	if res.Unspecified {
		if ips := listen.PrivateIPv4s(); len(ips) > 0 {
			host = ips[0]
		} else {
			host = "127.0.0.1"
		}
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d/", host, res.Port)
}

func loopbackURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// pairURLs builds the links to hand the browser. Loopback is omitted when any
// other bind address exists — pairing is for another device.
func pairURLs(res listen.Result, token string) []string {
	path := auth.PairPath + "?t=" + token
	hostPort := func(h string) string {
		if strings.Contains(h, ":") {
			h = "[" + h + "]"
		}
		return fmt.Sprintf("http://%s:%d%s", h, res.Port, path)
	}
	seen := map[string]bool{}
	var hosts []string
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			return
		}
		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			return
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	add(res.Host)
	for _, a := range res.BindAddrs() {
		h, _, err := net.SplitHostPort(a)
		if err != nil {
			add(a)
			continue
		}
		add(h)
	}
	if len(hosts) == 0 {
		return []string{hostPort("127.0.0.1")}
	}
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, hostPort(h))
	}
	return out
}

func printPairLinks(urls []string, expiresFmt string, minutes float64) {
	fmt.Println()
	for i, u := range urls {
		if i == 0 {
			fmt.Printf("    %s\n", u)
		} else {
			fmt.Printf("    %s   (alternate)\n", u)
		}
	}
	fmt.Println()
	fmt.Printf("Valid until %s (%.0f minutes), single use only.\n", expiresFmt, minutes)
}

// splitPositional pulls the first bare argument out of args so it can be
// handled separately from flags. It understands that -config consumes the
// value after it, so `-config path lan` does not mistake "path" for the tier.
func splitPositional(args []string) (positional string, rest []string) {
	valueTaking := map[string]bool{"-config": true, "--config": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			if valueTaking[a] && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		if positional == "" {
			positional = a
			continue
		}
		rest = append(rest, a)
	}
	return positional, rest
}
