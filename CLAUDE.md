# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Source of truth

**[`AGENT.md`](AGENT.md) is the single source of truth** for this repo: hard constraints, architecture, exact tmux/ssh argv, WebSocket protocol, config schema, and the "easy to break" list. This file does not restate that content — read AGENT.md before making any non-trivial change, and especially before touching listen-address policy, tmux argv, PTY lifetime, WebSocket framing, resize, or reconnect.

**Contract**: if a change alters behavior AGENT.md documents (a hard constraint, the protocol table, tmux/ssh argv, the config schema, REST routes, status codes), update AGENT.md in the same change. CLAUDE.md only needs updating when the *commands* below change (new build/test/lint step) — never duplicate AGENT.md's design content here.

Operator-facing setup (Tailscale, launchd, adding a remote host) is in [`README.md`](README.md). Design rationale is in [`docs/DESIGN.md`](docs/DESIGN.md) (Accepted, V2).

## Commands

```bash
go build -o homebase ./cmd/homebase   # build
go test ./...                          # run all tests
go test ./internal/tmux/...            # run one package's tests
go test ./internal/tmux/ -run TestName # run a single test
go test -race ./internal/devices/      # pairing is concurrency-sensitive
./scripts/vendor-xterm.sh              # re-vendor xterm.js (rare, not part of build)
```

Runtime subcommands (see AGENT.md, docs/DISTRIBUTION.md):

```bash
./homebase start                       # launchd / systemd user unit, then return
./homebase stop
./homebase restart                     # after Settings → Access
./homebase status                      # running?, bind, URL, tmux, version
./homebase pair                        # one-time 10-min login URL
./homebase version
./homebase serve                       # foreground HTTP (debug; not in help)
```

No frontend build step — `web/` is plain HTML/CSS/JS embedded via Go `embed` (`web/embed.go`).

## Architecture (orientation only — see AGENT.md for the full contract)

One Go binary. Browser connects over Tailscale to a fixed tmux session named `homebase` on the target host; the UI's window list is that session's tmux windows. tmux holds all state — Homebase is a PTY bridge, not a state store.

```
cmd/homebase/     start/stop/restart/status/pair/version + serve
internal/config/  hosts CRUD, atomic JSON write, schema v3, access tier
internal/listen/  tier → bind address → derived auth/TLS posture
internal/auth/    device-session gate, /pair redemption, Host pinning
internal/devices/ pairing tokens + enrolled devices (devices.json)
internal/tlsgen/  self-signed certificate management
internal/api/     REST: /api/hosts, /api/hosts/{id}/windows
internal/tmux/    control channel (list/new/rename/kill/select windows)
internal/session/ PTY channel: LocalDialer (exec tmux) + SSHDialer
internal/ws/      WebSocket upgrade, framing, resize
internal/ident/   validation for host/user/label/window name/index
web/              go:embed'd frontend + vendored xterm.js 5.5.0
```

Two channels per host: the **PTY channel** (one WebSocket, attached to the tmux session, carries raw bytes) and the **control channel** (short-lived `tmux` subprocess calls for list/new/rename/kill/select — never writes to the PTY). Local host (`id: "local"`) never goes through ssh; remote hosts always do.

**Security model in one line**: `access` picks the bind address, and auth + TLS are *derived* from that address — there is no config that expresses "routable, no credential, plaintext". The only credential is a device cookie enrolled by `homebase pair`, whose prerequisite (running a command on the host) is exactly what Homebase grants, so enrollment escalates nothing.

Before changing any of this, read AGENT.md — it has the exact argv, the WS frame table, the config schema, and a list of known failure modes (`Easy to break`).
