package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yanghanqing/homebase/internal/api"
	"github.com/yanghanqing/homebase/internal/auth"
	"github.com/yanghanqing/homebase/internal/projects"
	"github.com/yanghanqing/homebase/internal/session"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("homebase serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.json (default: ~/.config/homebase/config.json)")
	listenFlag := fs.String("listen", "", "bind host or host:port (cannot escape the access tier)")
	portFlag := fs.Int("port", 0, "listen port (default 1990)")
	logLevel := fs.String("log-level", "info", "debug|info|warn|error")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	level, ok := parseLevel(*logLevel)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown -log-level %q\n", *logLevel)
		return 2
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	if err := requireTmux(); err != nil {
		return 2
	}

	r, err := setup(*configPath, *listenFlag, *portFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	res := r.res

	projStore, err := projects.Load(filepath.Join(r.store.Dir(), "projects.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "projects:", err)
		return 1
	}

	if res.TierFallback {
		log.Warn("access is private but no address inside the trusted ranges was found; binding loopback — other devices cannot connect", "addr", res.Addr)
	}
	if res.NeedsAuth() && !res.Trusted {
		log.Warn("this listener is plain HTTP and NOT on a trusted range — see the security warning in Settings/README before exposing it to any network you don't fully trust", "addr", res.Addr)
	}

	var h http.Handler = api.NewMuxServer(&api.Server{
		Store:    r.store,
		Devices:  r.dev,
		Projects: projStore,
		Listen:   res.Addr,
		Dialer:   session.LocalDialer{},
		Log:      log,
		Version:  versionString(),
		Restart: func() {
			go func() {
				time.Sleep(400 * time.Millisecond)
				if err := kickRestart(); err != nil {
					log.Warn("settings saved but the service did not restart; run homebase restart", "err", err)
				}
			}()
		},
	})

	// Both postures are derived in listen.Result and read from it here. Do not
	// re-derive either one from the address: they are complements today, and
	// an `else` branch would silently pick the wrong one the day they stop
	// being complements.
	if res.NeedsAuth() {
		gate := &auth.Gate{Devices: r.dev, Log: log, Port: strconv.Itoa(res.Port)}
		h = gate.Wrap(h)
		list, _ := r.dev.List()
		log.Info("device pairing required", "paired", len(list))
		if len(list) == 0 {
			log.Warn("no devices paired yet — run 'homebase pair' to get a login link")
		}
	}
	if res.NeedsHostCheck() {
		// Loopback is unauthenticated, so the Host header is the only thing
		// standing between a malicious web page and a shell here.
		h = auth.RequireLoopbackHost(strconv.Itoa(res.Port), h)
		log.Info("loopback only: no pairing required, Host header pinned")
	}

	addrs := res.BindAddrs()
	srvs := make([]*http.Server, 0, len(addrs))
	errCh := make(chan error, len(addrs))
	for _, addr := range addrs {
		addr := addr
		srv := &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
		}
		srvs = append(srvs, srv)
		go func() {
			log.Info("listening",
				"addr", addr,
				"access", string(r.cfg.Access),
				"url", baseURL(res),
				"trusted_range", res.Trusted,
				"auth", res.NeedsAuth(),
				"config", r.store.Path(),
			)
			errCh <- srv.ListenAndServe()
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, srv := range srvs {
			_ = srv.Shutdown(shut)
		}
		return 0
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "http: %v\n", err)
			return 1
		}
		return 0
	}
}

func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return slog.LevelInfo, false
}
