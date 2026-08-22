package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yanghanqing/homebase/internal/config"
	"github.com/yanghanqing/homebase/internal/devices"
)

func runPair(args []string) int {
	fs := flag.NewFlagSet("homebase pair", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.json")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	r, err := setup(*configPath, "", 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !r.res.NeedsAuth() {
		fmt.Fprintf(os.Stderr, "access is %q, which binds loopback only — no pairing needed.\n", r.cfg.Access)
		fmt.Fprintf(os.Stderr, "Open %s on this machine, or change Access in Settings.\n", baseURL(r.res))
		return 1
	}

	token, expires, err := r.dev.Mint(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "mint: %v\n", err)
		return 1
	}

	fmt.Println()
	fmt.Println("Open this link on the device you want to pair:")
	printPairLinks(pairURLs(r.res, token), expires.Format("15:04:05"), devices.TokenTTL.Minutes())
	if !r.res.Trusted {
		fmt.Println()
		fmt.Println("WARNING: this link is unencrypted HTTP. On a regular LAN, other devices")
		fmt.Println("on the same network may intercept it. Prefer Tailscale or WireGuard.")
	}
	fmt.Println()
	return 0
}

func runDevices(args []string) int {
	fs := flag.NewFlagSet("homebase devices", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.json")
	revoke := fs.String("revoke", "", "device id to revoke")
	all := fs.Bool("revoke-all", false, "revoke every device and pending token")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	path := *configPath
	if path == "" {
		path = config.DefaultPath()
	}
	dev, err := devices.Open(devices.DefaultPath(path))
	if err != nil {
		fmt.Fprintf(os.Stderr, "devices: %v\n", err)
		return 1
	}
	switch {
	case *all:
		if err := dev.RevokeAll(); err != nil {
			fmt.Fprintf(os.Stderr, "revoke: %v\n", err)
			return 1
		}
		fmt.Println("All devices revoked.")
		return 0
	case *revoke != "":
		if err := dev.Revoke(*revoke); err != nil {
			fmt.Fprintf(os.Stderr, "revoke: %v\n", err)
			return 1
		}
		fmt.Printf("Revoked %s.\n", *revoke)
		return 0
	}

	list, err := dev.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "devices: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		fmt.Println("No devices paired yet. Run 'homebase pair' to get a login link.")
		return 0
	}
	for _, d := range list {
		fmt.Printf("%s  %s  %s\n", d.ID, d.CreatedAt.Format("2006-01-02 15:04"), d.Name)
	}
	return 0
}

func runAccess(args []string) int {
	tier, rest := splitPositional(args)

	fs := flag.NewFlagSet("homebase access", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.json")
	if code, ok := parseFlags(fs, rest); !ok {
		return code
	}
	if extra := fs.Args(); len(extra) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q\n", extra[0])
		return 2
	}
	path := *configPath
	if path == "" {
		path = config.DefaultPath()
	}
	store, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	if tier == "" {
		fmt.Println(store.Snapshot().Access)
		return 0
	}
	want := config.Access(strings.ToLower(tier))
	if !config.ValidAccess(want) {
		fmt.Fprintf(os.Stderr, "unknown access tier %q (want private or lan)\n", tier)
		return 2
	}
	if err := store.SetAccess(want); err != nil {
		fmt.Fprintf(os.Stderr, "access: %v\n", err)
		return 1
	}
	fmt.Printf("access = %s. Run 'homebase restart' for this to take effect.\n", want)
	if want == config.AccessLAN {
		fmt.Println("WARNING: lan binds every private IPv4 on this machine over unencrypted HTTP.")
		fmt.Println("On a regular LAN, other devices may intercept traffic. Prefer Tailscale or WireGuard.")
	}
	fmt.Println("After restarting, run 'homebase pair' to get a login link for each device.")
	return 0
}
