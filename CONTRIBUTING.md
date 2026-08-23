# Contributing to Homebase

Thanks for taking a look. Homebase is deliberately small: one Go binary, two
dependencies, no frontend build step. Most of what follows exists to keep it
that way.

## Before you open a pull request

**Security bugs do not go here.** Homebase hands a browser a shell, so a flaw
in the auth gate, the bind-address policy, or the pairing flow is a remote code
execution bug. Report those privately — see [SECURITY.md](SECURITY.md).

**Check the scope first.** Homebase drives *one* tmux session on *the local
machine*. These are settled design decisions, not gaps waiting to be filled:

- No multi-host, no ssh, no built-in TLS. Transport encryption is the overlay
  network's job (Tailscale, WireGuard).
- No accounts, no passwords, no OAuth. The only credential is a device cookie
  minted by `homebase pair`.
- No Electron/Tauri, no React/Vue/Svelte, no Node.js at runtime, no CDN.
- No more than one tmux session, and no split-pane UI of our own — tmux does
  splits.
- No new Go dependency without a very good reason. An unused module is not
  free; it still shows up in vulnerability scans, and CI fails on a dirty
  `go mod tidy`.

If you want one of those anyway, open an issue and make the case before
writing code. A PR that adds one will be closed regardless of how good the
implementation is.

## Development

Requires **Go 1.24+** and **tmux** (a runtime dependency, not a build one).

```bash
go build -o homebase ./cmd/homebase   # build
go test ./...                          # all tests
go test ./internal/tmux/...            # one package
go test -race ./internal/devices/      # pairing is concurrency-sensitive
```

There is no frontend build step. `web/` is plain HTML/CSS/JS embedded with Go
`embed`, and xterm.js is vendored in-tree.

Run the server in the foreground while you work:

```bash
./homebase serve
```

### What CI checks

Push to `main` and every pull request runs, on both Linux and macOS:

```bash
gofmt -l .          # must print nothing
go mod tidy         # must leave go.mod and go.sum unchanged
go vet ./...
go test -race ./...
govulncheck ./...
```

Run those locally before pushing. `go test -race ./...` in one shot can get
OOM-killed on a laptop; run it per package if that happens.

CI installs tmux so the live tmux tests actually execute. They skip themselves
when tmux is missing *or* when they cannot get a private tmux server, and a
skip looks exactly like a pass unless you read the output — so after touching
anything under `internal/tmux`, run `go test -v ./internal/tmux/` and confirm
it says PASS rather than SKIP.

## Style

- Match the surrounding code. Comments explain *why*, not *what*.
- **Commit messages are English**, with an imperative subject: `Add launchd
  start`, not `Added start command`. Body, if any, is English too.
- Keep pull requests focused. One behavior change per PR is much easier to
  review than five.
- If a change alters something the README or SECURITY.md describes, update
  those documents in the same commit.

## Releases

Releases are cut by GitHub Actions from an annotated tag `vX.Y.Z` — never from
a laptop. Maintainers only.
