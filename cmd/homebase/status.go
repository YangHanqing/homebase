package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/yanghanqing/homebase/internal/tmux"
)

func runStatus(args []string) int {
	fs := flag.NewFlagSet("homebase status", flag.ContinueOnError)
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

	p := probeLoopback(r.res.Port)
	running := p.kind == probeOurs || (serviceProbeFallback(r) && serviceActive())
	if running {
		fmt.Println("running        yes")
	} else {
		fmt.Println("running        no")
	}

	if path, err := tmux.LocalBinary(); err != nil {
		fmt.Println("tmux           not found")
	} else {
		fmt.Printf("tmux           %s\n", path)
	}
	fmt.Printf("version        %s\n", versionString())
	fmt.Printf("log            %s\n", logLocation())

	list, _ := r.dev.List()
	fmt.Printf("access         %s\n", r.cfg.Access)
	fmt.Printf("bind           %s\n", strings.Join(r.res.BindAddrs(), " "))
	urls := offLoopbackURLs(r.res)
	if len(urls) == 0 {
		fmt.Printf("url            %s\n", baseURL(r.res))
	} else {
		fmt.Printf("url            %s\n", strings.Join(urls, " "))
	}
	fmt.Printf("trusted range  %v\n", r.res.Trusted)
	fmt.Printf("pairing        %v\n", r.res.NeedsAuth())
	fmt.Printf("devices        %d\n", len(list))
	fmt.Printf("config         %s\n", r.store.Path())
	if r.res.TierFallback {
		fmt.Println("warning        access=private but no address inside the trusted ranges was found; fell back to loopback")
	}
	if r.res.NeedsAuth() && !r.res.Trusted {
		fmt.Println("warning        plain HTTP on an untrusted range — see README security notes")
	}
	if p.kind == probeOurs && p.listen != "" && p.listen != r.res.Addr {
		fmt.Println("warning        config changed since start — run 'homebase restart'")
	}
	return 0
}
