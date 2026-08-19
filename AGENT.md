# Homebase — Agent Guide

Self-hosted web terminal session manager. One Go binary on an always-on home machine (Mac mini). Browser over Tailscale attaches to tmux on every house host. Closing the browser, sleeping a laptop, or a Wi-Fi blip must not lose work: **tmux holds state, Homebase is a dumb PTY bridge.**

Canonical design: [`docs/DESIGN.md`](docs/DESIGN.md) (Accepted). Implementation order: [`docs/PLAN.md`](docs/PLAN.md). Operator notes: [`README.md`](README.md).

Read DESIGN.md before changing listen-address policy, SSH argv, PTY lifetime, WebSocket framing, resize, or reconnect.

## Hard constraints

These are not style preferences. If a change fights them, do not ship it.

1. **Never bind `0.0.0.0`, `::`, or an unspecified address** unless `allow_public_bind` is explicitly `true` in config. Default is Tailscale IPv4 if `tailscale ip -4` works, else `127.0.0.1`. `-listen` cannot bypass this gate.
2. **Never accept, store, prompt for, or log SSH passwords or keys.** Key-based auth only. `BatchMode=yes`. No `sshpass`, no `StrictHostKeyChecking=no`.
3. **Never kill a remote tmux server or session.** Window delete and WebSocket close kill only the local `ssh` process (the tmux *client*). No `tmux kill-session`. No `tmux -D`.
4. **Never log PTY contents, tokens, bcrypt hashes, or Authorization headers.**
5. **PTY bytes stay bytes.** Do not decode stdout as UTF-8 Go `string` or JS `string` on the data path. xterm.js gets `Uint8Array`. stdin is raw bytes.
6. **`exec.Command` with separate argv.** Never `bash -c` interpolating user fields. Host / user / session_name are validated then passed as arguments.
7. **Remote command is shell-agnostic:** one `exec /bin/sh -c '…'` (POSIX inside). Remote login shell may be fish/csh. See DESIGN.md §4.3.
8. **No Electron, no Node.js at runtime, no React/Vue/Svelte.** Vanilla HTML/CSS/JS + vendored xterm.js 5.5.0. Go `embed` for the UI. A one-time vendor script may use curl/npm; the deployed artifact is one binary.
9. **No `tmux -CC`.** Homebase shows a normal tmux client (status bar, `C-b`, panes) inside xterm.js.
10. **Page load auto-connects every saved Window.** Users must not click each row to restore sessions.
11. **Resize is correctness.** FitAddon → WS `{type:resize}` → `pty.Setsize`. No hardcoded rows/cols.
12. **Single-user, no accounts.** Optional Basic Auth for the HTTP server itself, default off.

## Stack

| Item | Value |
| --- | --- |
| Language | Go 1.22+ |
| Module | `github.com/yanghanqing/homebase` |
| PTY | `github.com/creack/pty` |
| WebSocket | `github.com/gorilla/websocket` |
| Frontend | HTML/CSS/JS + xterm.js 5.5.0 + `@xterm/addon-fit`, vendored, no CDN at runtime |
| Fonts | system-ui / ui-monospace; do not vendor font files |
| Config | `~/.config/homebase/config.json` (override `-config`), atomic write |
| Auth | optional HTTP Basic Auth (bcrypt), default off |
| Deploy | one static binary, `CGO_ENABLED=0` if possible (`creack/pty` is syscall-based, still works with CGO off on macOS/Linux) |

Default port: **7681**.

## Layout

```
homebase/
  AGENT.md                 this file
  README.md                operator: Tailscale, ssh-copy-id, tmux, run
  docs/DESIGN.md           canonical technical design (Accepted)
  docs/PLAN.md             ordered implementation PRs
  go.mod
  cmd/homebase/main.go     flags, listen, graceful shutdown
  internal/config/         load/save/validate, atomic write
  internal/listen/         Tailscale detect, public-bind guard
  internal/auth/           optional Basic Auth middleware
  internal/api/            REST /api/windows
  internal/session/        PTY + ssh lifecycle per Window
  internal/ws/             WebSocket upgrade, framing, resize
  internal/ident/          host/user/session_name validation
  web/                     go:embed
    index.html
    css/app.css
    js/app.js              window list, CRUD, auto-connect
    js/session.js          per-window WS + backoff
    js/terminal.js         xterm + FitAddon + resize
    vendor/xterm/          pinned xterm.js 5.5.0 + css + fit addon
  scripts/vendor-xterm.sh  download pinned files into web/vendor
  internal/*/…_test.go
```

Do not add `pkg/`. Do not add a frontend build step to `go build`.

## Architecture

```
Browser (any Tailscale device)
  GET  /                 embedded UI
  REST /api/windows      CRUD config (source of truth on disk)
  WS   /ws/windows/{id}  one PTY bridge per Window per browser
           │
     Homebase process
           │  ssh -tt -o BatchMode=yes user@host  'exec /bin/sh -c … tmux new-session -A -s NAME'
           ▼
     remote tmux session (the persistent thing)
```

**Model A — PTY is tied to the WebSocket.** On WS connect, spawn ssh+pty. On WS close, kill the ssh process group. Reconnect runs the same command again (`tmux new-session -A` is idempotent; scrollback comes from tmux). Do **not** keep a server-side PTY alive with no client and multiplex it.

Two browsers on the same Window ⇒ two ssh clients attached to the same tmux session. That is correct tmux behavior. Do not add `-D` (would kick the other client). Size wars are tmux's problem (`window-size` / `aggressive-resize` in the user's tmux.conf).

### Named types (responsibilities)

| Type | Package | Job |
| --- | --- | --- |
| `File` / `Store` | `internal/config` | JSON file, mutex, atomic replace, schema `version` |
| `Window` | `internal/config` | `id`, `name`, `host`, `user`, `session_name`, `order` |
| `Validator` | `internal/ident` | reject `;|&$\` \n` and anything that is not a safe host/user/session |
| `Dialer` | `internal/session` | interface: `Start(ctx, window, size) (Proc, error)` |
| `SSHDialer` | `internal/session` | production: `/usr/bin/ssh` + `creack/pty` |
| `Proc` | `internal/session` | `Read`, `Write`, `SetSize`, `Wait`, `Kill` |
| `Hub` | `internal/ws` | one WS per request; pipes Proc ↔ browser |
| `Backoff` | frontend `session.js` | 1s → 2s → 4s → … cap 30s, reset on `connected` |

Tests inject a `Dialer` that runs local `tmux` (no ssh). Production V1 always ssh. No `transport: local`.

## Protocol

WebSocket URL: `GET /ws/windows/{id}` (same origin). Unknown id: close **4404** with reason `unknown window`. No connection-count limit and no 4403.

| Direction | Frame | Payload |
| --- | --- | --- |
| client → server | binary | raw stdin bytes |
| client → server | text JSON | `{"type":"resize","cols":120,"rows":40}` |
| client → server | text JSON | `{"type":"ping"}` |
| server → client | binary | raw stdout bytes |
| server → client | text JSON | `{"type":"status","state":"connecting\|connected\|disconnected\|error","code":"…","message":"…"}` |
| server → client | text JSON | `{"type":"pong"}` |

Unknown JSON `type`: ignore. Malformed JSON: log at debug, ignore. Resize with cols/rows `< 1` or `> 512`: ignore.

On reconnect the frontend calls `term.reset()` then opens a new WS. tmux redraws. Do not try to replay a local xterm buffer.

### Status codes (`code` field)

| code | Meaning | Typical UI |
| --- | --- | --- |
| `ssh_auth` | Permission denied | key not accepted; remind ssh-copy-id |
| `ssh_hostkey` | Host key verification failed | ssh to that host once from the Homebase machine |
| `ssh_timeout` | ConnectTimeout | host down / Tailscale |
| `ssh_refused` | connection refused | sshd not listening |
| `enotmux` | remote printed `HOMEBASE_ENOTMUX` | install tmux on remote; session was never there, or died with tmux |
| `pty_spawn` | local spawn failed | binary/path problem |
| `ws_closed` | socket gone | disconnected / retrying |
| `unknown` | anything else | show trimmed ssh stderr (no secrets) |

## SSH argv (do not improvise)

Binary: `/usr/bin/ssh` so `~/.ssh/config`, `known_hosts`, agent, ProxyJump, ControlMaster, Tailscale hostnames all work.

Flags (fixed):

```
-tt
-o BatchMode=yes
-o NumberOfPasswordPrompts=0
-o ConnectTimeout=10
-o ServerAliveInterval=15
-o ServerAliveCountMax=3
```

Then `{user}@{host}`, then one remote command argument:

```
exec /bin/sh -c 'PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/opt/local/bin"; export PATH; t=`command -v tmux 2>/dev/null`; if [ -z "$t" ]; then echo HOMEBASE_ENOTMUX; exit 127; fi; exec "$t" new-session -A -s '"$quotedSession"
```

Rules:

- Outer command is exactly `exec /bin/sh -c '…'` so fish/csh/zsh all work.
- Inner script is POSIX. Use backticks not `$( )` (csh interpolates `$(` even inside some quote styles).
- Session name is regex-validated **and** POSIX single-quoted before interpolation.
- **No** `tmux -D`. **No** `tmux -CC`. **No** `kill-session`.
- After `creack/pty.Start`, set the winsize **before** relying on a TUI. Kill via process group (`Setpgid` / `Setsid`) so ssh's children die with it.

### Validation

| Field | Rule |
| --- | --- |
| `name` | 1–64 chars, trimmed, no control chars |
| `host` | DNS label / IPv4 / IPv6 / Tailscale MagicDNS. No space, `@`, `;`, `|`, `&`, `$`, `` ` ``, newline |
| `user` | `^[A-Za-z0-9._-]+$`, 1–32 |
| `session_name` | `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$` |

Reject on API write and again immediately before `exec.Command`.

## Listen policy

1. Flag `-listen` / config `listen_addr` if set (host or host:port).
2. Else run `tailscale ip -4` with a 2s timeout (look in `PATH`, then `/opt/homebrew/bin/tailscale`, `/usr/local/bin/tailscale`). If it prints a unicast IPv4, use it.
3. Else `127.0.0.1`, and log that other devices cannot connect.
4. Port from config/`-port`, default `7681`.
5. If the resolved address is unspecified (`0.0.0.0`, `::`, empty host with `:7681`), **refuse to start** unless `allow_public_bind: true`.

Log the actual bound address once at INFO.

## Frontend

- One xterm.js instance **and** one WebSocket per Window, created on page load for every config row.
- Right pane shows one terminal at a time. Inactive wrappers stay in the DOM at the pane's size. Prefer `visibility: hidden` (still laid out) over `display: none` (FitAddon → 0×0). On show: `fit()` and send resize.
- All Windows, including hidden ones, get the **current pane size** on first connect (same box).
- `ResizeObserver` on the pane, debounce ~32ms, `fit()` + resize message for the **visible** terminal; hidden ones get the size too so a later switch is not 80×24.
- Reconnect: 1s → 2s → 4s → 8s → 16s → 30s, stay at 30s, never stop. Reset delay on `state=connected`.
- CRUD hits REST then opens/closes that row's WS immediately. No full reload required. DELETE does not tell the remote to kill tmux.
- Fonts: `system-ui` and `ui-monospace`. Do not vendor `.woff` / `.woff2`.

## Config schema (v1)

Path: `$XDG_CONFIG_HOME/homebase/config.json` → `~/.config/homebase/config.json`. Flag `-config` overrides.

If the file is missing on startup: create the parent directory `0700` if needed, atomically write this default with mode `0600`, then continue. Do not seed any Window.

```json
{
  "version": 1,
  "listen_addr": "",
  "listen_port": 7681,
  "allow_public_bind": false,
  "auth": {
    "enabled": false,
    "username": "",
    "password_bcrypt": ""
  },
  "windows": []
}
```

A populated file looks like:

```json
{
  "version": 1,
  "listen_addr": "",
  "listen_port": 7681,
  "allow_public_bind": false,
  "auth": {
    "enabled": false,
    "username": "",
    "password_bcrypt": ""
  },
  "windows": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Home Mac mini",
      "host": "mac-mini",
      "user": "yourname",
      "session_name": "work",
      "order": 1
    }
  ]
}
```

- `id` is server-generated UUID. POST ignores client-supplied ids.
- Write: temp file in the same directory, `fsync`, `rename`. Mutex around read-modify-write.
- Unknown JSON fields: ignore (forward compat). Missing `version`: treat as 1.
- File exists but corrupt, or `version` > 1: refuse to start; do not overwrite.

REST (JSON, same origin):

```
GET    /api/windows
POST   /api/windows          {name,host,user,session_name}
GET    /api/windows/{id}
PUT    /api/windows/{id}     replace fields; if host/user/session_name change, UI reconnects
DELETE /api/windows/{id}     config only
PUT    /api/windows/reorder  {ids: [...]}
GET    /api/health           {ok:true, listen:"…"}
```

Mutating requests: require `Origin` header host to match request host (CSRF belt-and-suspenders for Basic Auth). Missing Origin (curl) is allowed.

Cap: **32** windows. POST past the cap returns 400.

## Auth (optional)

Default off. When `auth.enabled`:

- HTTP Basic Auth on every route, including WS upgrade.
- Password stored as bcrypt. Compare with `bcrypt.CompareHashAndPassword`. Username constant-time compare.
- Browser already sent Basic for `GET /`, so `new WebSocket(sameOrigin)` reuses it. Do not put tokens in the WS query string.
- `homebase hash-password` subcommand writes a bcrypt hash to stdout so the operator can paste it into config. Do not add a password in argv to a long-running process.

## Easy to break

- **Homebrew tmux PATH.** `ssh host tmux` is a non-login shell; `/opt/homebrew/bin` is missing. The `/bin/sh` PATH prefix is mandatory.
- **Fish/csh as login shell.** Any `{ …; }` or `$( )` in the remote string will misparse. Do not "simplify" the remote command.
- **`display:none` + FitAddon.** Inactive terminals report 0×0; the first TUI is garbage until a manual resize. Keep layout size.
- **Go `string(ptyBytes)`.** Invalid UTF-8 in TUI output will corrupt the stream. Copy `[]byte`.
- **Killing tmux instead of ssh.** `cmd.Process.Kill()` must be the local ssh. Never send `tmux kill-server` / `kill-session`.
- **`-D` on attach.** Detaches other clients (another browser, an iPad). Forbidden.
- **First-time host keys.** `BatchMode=yes` cannot answer `yes/no`. Surface `ssh_hostkey` and tell the operator to `ssh user@host` once **on the Homebase machine**.
- **Logging stderr from ssh.** Fine if trimmed; never at a level that dumps a password prompt (BatchMode should prevent prompts).

## Tests (required as you build)

Run from repo root: `go test ./...`

Must exist by the time the matching PR lands:

- `ident`: reject `; rm -rf`, `$(reboot)`, newlines, empty, oversize
- `config`: round-trip, atomic write, missing file writes default, corrupt file error, cap 32
- `listen`: `0.0.0.0` refused without `allow_public_bind`; Tailscale parse; fallback
- `session`: local `tmux` Dialer start/resize/write/kill does **not** leave a leaked pane process (session may remain — that is tmux)
- `ws` protocol: resize JSON, binary vs text, ignore unknown type, unknown id closes 4404
- frontend logic if extracted: backoff sequence `1,2,4,8,16,30,30`

Do not require a second physical host for CI. Live ssh is a manual probe, not a unit test.

## What not to do

- Multi-user accounts, OAuth, SSO
- Electron / Tauri / Wails
- SFTP, file upload, paste-upload of files (clipboard paste of text is fine)
- Session recording, replay, AI, split-pane UI of our own (tmux does splits)
- SQLite unless JSON is proven insufficient (it is sufficient)
- CDN for xterm.js or fonts
- Binding all interfaces "just for convenience"
- A WebSocket connection-count limiter
- `transport: local` in V1

## Build

```bash
go build -o homebase ./cmd/homebase
go test ./...
```

Vendor UI deps (when adding/updating xterm):

```bash
./scripts/vendor-xterm.sh
```
