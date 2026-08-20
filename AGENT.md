# Homebase — Agent Guide

Self-hosted web terminal. One Go binary on an always-on home machine (Mac mini). Browser over Tailscale attaches to **one fixed tmux session per host, named `homebase`**. The sidebar lists the **tmux windows** inside that session. Closing the browser, sleeping a laptop, or a Wi-Fi blip must not lose work: **tmux holds state, Homebase is a dumb PTY bridge.**

Canonical design: [`docs/DESIGN.md`](docs/DESIGN.md) (Accepted, V2). Operator notes: [`README.md`](README.md). Install / CLI / service lifecycle: [`docs/DISTRIBUTION.md`](docs/DISTRIBUTION.md).

Read DESIGN.md before changing listen-address policy, tmux argv, PTY lifetime, WebSocket framing, resize, or reconnect.

## Hard constraints

These are not style preferences. If a change fights them, do not ship it.

1. **Auth and TLS are derived from the bind address, never configured separately.** Off-loopback ⇒ pairing required. Off-loopback and outside `100.64.0.0/10` ⇒ TLS required. There is no switch that turns either off, and `-listen` cannot escape the `access` tier. See §Access tiers.
2. **Never accept, store, prompt for, or log SSH passwords or keys.** Key-based auth only. `BatchMode=yes`. No `sshpass`, no `StrictHostKeyChecking=no`.
3. **Never kill a tmux server or session.** `kill-window` is allowed for UI window delete, but **refuse when it is the last window** (that would destroy the session) — enforce in the API, not just the UI. WebSocket close kills only the local process (the tmux *client* / `ssh`). No `tmux kill-session`. No `tmux kill-server`. No `tmux -D`.
4. **Never log PTY contents, pairing tokens, session secrets, cookies, or Authorization headers.**
5. **PTY bytes stay bytes.** Do not decode stdout as UTF-8 Go `string` or JS `string` on the data path. xterm.js gets `Uint8Array`. stdin is raw bytes.
6. **`exec.Command` with separate argv.** Never `bash -c` interpolating user fields. Host / user / window name / index are validated then passed as arguments.
7. **Remote command is shell-agnostic:** one `exec /bin/sh -c '…'` (POSIX inside). Remote login shell may be fish/csh. See DESIGN.md §4.3.
8. **The control channel never writes to the PTY.** Listing / creating / renaming / killing / selecting windows runs a separate short-lived `tmux` process. Injecting `C-b` bytes into the PTY would type into whatever the user is running.
9. **No `tmux -CC`.** Homebase shows a normal tmux client inside xterm.js.
10. **No Electron, no Node.js at runtime, no React/Vue/Svelte.** Vanilla HTML/CSS/JS + vendored xterm.js 5.5.0. Go `embed` for the UI. A one-time vendor script may use curl/npm; the deployed artifact is one binary.
11. **Page load auto-connects.** One terminal, connected immediately. The user must not have to click anything to get a shell.
12. **Resize is correctness.** FitAddon → WS `{type:resize}` → `pty.Setsize`. No hardcoded rows/cols.
13. **Single-user, no accounts, and no password.** The only credential is a device session minted by `homebase pair`, which requires the ability to run a command on the host — the same ability Homebase hands out. Enrollment must never grant more than the enroller already had.
14. **The local host is not configurable.** It has id `local`, always exists, cannot be created, edited, or deleted, and never goes through ssh.
15. **Never log a pairing token, a session secret, or a cookie value.** `devices.json` stores SHA-256 hashes only.
16. **The unauthenticated loopback tier must pin the `Host` header.** Without it, DNS rebinding gives any web page a shell. This check is what makes "local needs no password" true rather than merely convenient.
17. **CGNAT membership decides TLS only — never identity.** An address cannot prove who is calling; treating it as authentication would be a silent auth bypass.
18. **Commit messages are English.** Imperative subject (`Add launchd start`, not "添加了 start" or "Added start command"). Body, if any, is English too. This includes fixups and tag annotations.
19. **Releases are GitHub Actions, never a laptop.** Ship by pushing an annotated tag `vX.Y.Z`. Do not run `goreleaser release` locally and do not attach Release assets by hand. A tag without the `v` prefix does not trigger CI.

## Stack

| Item | Value |
| --- | --- |
| Language | Go 1.22+ |
| Module | `github.com/yanghanqing/homebase` |
| PTY | `github.com/creack/pty` |
| WebSocket | `github.com/gorilla/websocket` |
| Frontend | HTML/CSS/JS + xterm.js 5.5.0 + `addon-fit` + `addon-unicode11`, vendored, no CDN at runtime |
| Fonts | system-ui / ui-monospace; do not vendor font files |
| Config | `~/.config/homebase/config.json` (override `-config`), atomic write, schema **version 3** |
| Auth | device session cookie, enrolled by one-time `homebase pair` token. No password. |
| TLS | self-signed, generated and managed by Homebase; required off-loopback outside CGNAT |
| Deploy | one static binary |

Default port: **1990**. Fixed tmux session name: **`homebase`**.

## Layout

```
homebase/
  AGENT.md                 this file
  README.md                operator: Tailscale, tmux, run
  docs/DESIGN.md           canonical technical design (Accepted, V2)
  go.mod
  cmd/homebase/            start/stop/restart/status/pair/version; serve (foreground)
  install.sh               curl installer (GitHub Release asset; do not compile)
  .goreleaser.yaml         asset names are a contract with install.sh
  .github/workflows/       tag `v*` → GoReleaser (the only release path)
  internal/config/         load/save/validate, hosts CRUD, atomic write, access tier
  internal/listen/         tier → bind address → derived auth/TLS posture
  internal/auth/           device-session gate, /pair redemption, Host pinning
  internal/devices/        pairing tokens + enrolled devices (devices.json)
  internal/tlsgen/         self-signed certificate management
  internal/api/            REST /api/windows, /api/settings, /api/devices
  internal/tmux/           control channel: list/new/rename/kill/select
  internal/session/        PTY channel: LocalDialer (exec tmux) + SSHDialer
  internal/ws/             WebSocket upgrade, framing, resize
  internal/ident/          host/user/label/window-name/index validation
  web/                     go:embed
    index.html
    css/app.css
    js/app.js              host switcher, window list, CRUD
    js/session.js          the single WS + backoff
    js/terminal.js         xterm + FitAddon + resize
    vendor/xterm/          pinned xterm.js 5.5.0 + css + addons
  scripts/vendor-xterm.sh
  internal/*/…_test.go
```

Do not add `pkg/`. Do not add a frontend build step to `go build`.

## Architecture

```
Browser (any Tailscale device)
  GET  /                          embedded UI
  REST /api/hosts                 CRUD remote hosts (config on disk)
  REST /api/hosts/{id}/windows    tmux window list / new / rename / kill / select
  WS   /ws/hosts/{id}             ONE PTY bridge, attached to session `homebase`
           │
     Homebase process
           ├─ local:  exec tmux …
           └─ remote: ssh -tt -o BatchMode=yes user@host 'exec /bin/sh -c … tmux …'
           ▼
     tmux session `homebase` (the persistent thing)
```

**Model A — PTY is tied to the WebSocket.** On WS connect, spawn. On WS close, kill the local process. Reconnect runs the same command again (`tmux new-session -A` is idempotent; scrollback comes from tmux). Do **not** keep a server-side PTY alive with no client.

Two browsers on the same host ⇒ two tmux clients on the same session. That is correct tmux behavior. Do not add `-D`.

Switching host tears down the WS and opens a new one. Do not keep background connections.

### Named types (responsibilities)

| Type | Package | Job |
| --- | --- | --- |
| `File` / `Store` | `internal/config` | JSON file, mutex, atomic replace, schema `version` |
| `Host` | `internal/config` | `id`, `label`, `user`, `host`, `order` — remote only |
| `Validator` fns | `internal/ident` | reject `;\|&$\` \n` and anything unsafe |
| `Runner` | `internal/tmux` | interface: `Run(ctx, args…) ([]byte, error)` |
| `LocalRunner` / `SSHRunner` | `internal/tmux` | exec tmux directly / wrap in ssh |
| `Client` | `internal/tmux` | `ListWindows` / `NewWindow` / `RenameWindow` / `KillWindow` / `SelectWindow` |
| `Dialer` | `internal/session` | interface: `Start(ctx, target, size) (Proc, error)` |
| `LocalDialer` / `SSHDialer` | `internal/session` | PTY channel, local exec / `/usr/bin/ssh` |
| `Proc` | `internal/session` | `Read`, `Write`, `SetSize`, `Wait`, `Kill` |
| `Hub` | `internal/ws` | one WS per request; pipes Proc ↔ browser |
| `Backoff` | frontend `session.js` | 1s → 2s → 4s → … cap 30s, reset on `connected` |

## Protocol

WebSocket URL: `GET /ws/hosts/{id}` (same origin). Unknown id: close **4404** with reason `unknown host`. No connection-count limit.

| Direction | Frame | Payload |
| --- | --- | --- |
| client → server | binary | raw stdin bytes |
| client → server | text JSON | `{"type":"resize","cols":120,"rows":40}` |
| client → server | text JSON | `{"type":"ping"}` |
| server → client | binary | raw stdout bytes |
| server → client | text JSON | `{"type":"status","state":"connecting\|connected\|disconnected\|error","code":"…","message":"…"}` |
| server → client | text JSON | `{"type":"pong"}` |

Unknown JSON `type`: ignore. Malformed JSON: log at debug, ignore. Resize with cols/rows `< 1` or `> 512`: ignore.

On reconnect the frontend calls `term.reset()` then opens a new WS. tmux redraws.

### Status codes (`code` field)

| code | Meaning | Typical UI |
| --- | --- | --- |
| `ssh_auth` | Permission denied | key not accepted; remind ssh-copy-id (remote only) |
| `ssh_hostkey` | Host key verification failed | ssh to that host once from the Homebase machine |
| `ssh_timeout` | ConnectTimeout | host down / Tailscale |
| `ssh_refused` | connection refused | sshd not listening |
| `enotmux` | tmux not found locally or remotely | install tmux |
| `pty_spawn` | local spawn failed | binary/path problem |
| `ws_closed` | socket gone | disconnected / retrying |
| `unknown` | anything else | show trimmed stderr (no secrets) |

The local host can only produce `enotmux`, `pty_spawn`, `ws_closed`, `unknown`.

## tmux argv (do not improvise)

Session name is the compile-time constant `homebase`.

**PTY (attach):**

```
new-session -A -s homebase [-c DIR] ; set-option -t homebase status off
```

The `;` must be its **own quoted argv element**. Locally that means a literal `";"` argument; remotely it must be POSIX-single-quoted (`';'`) or `/bin/sh` will treat it as a command separator.

`-c DIR` is the session's start directory and is passed **local host only**, as the operator's `$HOME` resolved to an absolute path. Without it the session inherits the *homebase server process's* cwd, which under launchd is arbitrary. `-A` ignores `-c` when the session already exists, so passing it is always safe. Never pass a literal `~`: the argv never reaches a shell, so it would not be expanded. Remote hosts pass **no** `-c` — a non-interactive ssh command already starts in the remote `$HOME`, and a local path would be meaningless there.

**Control:**

```
display-message -p -t homebase '#{pane_current_path}'
list-windows  -t homebase -F '#{window_index} #{window_active} #{window_name}'
new-window    -t homebase [-c DIR] -P -F '#{window_index}'
rename-window -t homebase:IDX NAME
kill-window   -t homebase:IDX          (refuse if it is the last window)
select-window -t homebase:IDX
```

`list-windows` failing with `can't find session` is **not an error**: return an empty list. The PTY connect creates the session.

**A new window must open in the active window's directory.** `new-window` without `-c` inherits the cwd of *the process that ran it* — the homebase server — not the session's active pane, so every new window would land in the same arbitrary directory regardless of where the user is. So `NewWindow` first reads `#{pane_current_path}` via `display-message` and feeds it to `-c`. That lookup is **best-effort**: on any error (no session yet, tmux hiccup) it falls back to omitting `-c` rather than failing the create. Costs one extra tmux round-trip per new window.

**Never use a tab (or any control character) as the `-F` field separator.** When the process has no UTF-8 locale — exactly what launchd gives a user agent — tmux rewrites control characters in `-F` output as `_`, and the list silently parses to nothing. Machine fields first, free-form name last, single space between, parse with `SplitN(line, " ", 3)`.

Every exec'd command gets a UTF-8 locale (`LANG=en_US.UTF-8`) when the parent has none. Without it tmux and the shell mangle non-ASCII in the PTY too.

Do **not** turn off `automatic-rename`. tmux clears it per-window on manual `rename-window`, which is exactly the behavior we want.

## SSH argv (do not improvise)

Binary: `/usr/bin/ssh` so `~/.ssh/config`, `known_hosts`, agent, ProxyJump, Tailscale hostnames all work. **Remote hosts only** — the local host never uses ssh.

```
-tt                                  (PTY channel only; the control channel must NOT pass -tt)
-o BatchMode=yes
-o NumberOfPasswordPrompts=0
-o ConnectTimeout=10
-o ServerAliveInterval=15
-o ServerAliveCountMax=3
-o ControlMaster=auto
-o ControlPath=<tmpdir>/homebase-%C
-o ControlPersist=60
```

`ControlPath` is a unix socket: macOS caps it at 104 bytes. Build it from `os.TempDir()`, and fall back to `/tmp/homebase-%C` if it would be too long.

Then `{user}@{host}`, then one remote command argument:

```
exec /bin/sh -c 'PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/opt/local/bin"; export PATH; t=`command -v tmux 2>/dev/null`; if [ -z "$t" ]; then echo HOMEBASE_ENOTMUX; exit 127; fi; exec "$t" <each arg POSIX-single-quoted>'
```

Rules:

- Outer command is exactly `exec /bin/sh -c '…'` so fish/csh/zsh all work.
- Inner script is POSIX. Use backticks not `$( )` (csh interpolates `$(` even inside some quote styles).
- Every argument is validated **and** POSIX single-quoted before interpolation.
- After `creack/pty.Start`, set the winsize **before** relying on a TUI.

### Validation

| Field | Rule |
| --- | --- |
| `label` | 1–64 chars, trimmed, no control chars |
| `host` | DNS label / IPv4 / IPv6 / Tailscale MagicDNS. No space, `@`, `;`, `\|`, `&`, `$`, `` ` ``, newline |
| `user` | `^[A-Za-z0-9._-]+$`, 1–32 |
| window name | 1–64 chars, no control chars (spaces allowed — tmux allows them) |
| window index | `^[0-9]{1,4}$` |

Reject on API write and again immediately before `exec.Command`.

## Access tiers

`access` is the one security knob. It picks the bind address; **auth and TLS are
derived from that address and are not separately configurable.** A user cannot
construct the dangerous combination (routable address, no credential, plaintext)
because no config expresses it.

| `access` | Binds | TLS | Pairing | Host pinned |
| --- | --- | --- | --- | --- |
| `local` (default) | `127.0.0.1` | no | no | **yes** |
| `private` | Tailscale/CGNAT IPv4 | no (WireGuard) | yes | no |
| `lan` | `0.0.0.0` | **yes**, self-signed | yes | no |

Derivation, implemented once in `listen.Result`:

- `NeedsAuth()` = not loopback. Reaching loopback already implies a local shell, so a credential there guards nothing.
- `NeedsTLS()` = not loopback and not in `100.64.0.0/10`. The session cookie rides every request; a one-time pairing token does not protect the long-lived credential it mints.
- `NeedsHostCheck()` = loopback. Only the unauthenticated tier needs it, and there it is load-bearing.

Resolution:

1. `-listen` / `listen_addr` if set (host or host:port), else the tier's default address.
2. For `private`: scan interfaces for an IPv4 in `100.64.0.0/10` **first** (no subprocess, works regardless of how Tailscale was installed), then fall back to `tailscale ip -4` with a 2s timeout. Look in `PATH`, then `/opt/homebrew/bin/tailscale`, `/usr/local/bin/tailscale`, then `/Applications/Tailscale.app/Contents/MacOS/Tailscale` — the App Store build never puts its CLI on `PATH`.
3. `private` with no CGNAT address found: bind loopback, set `TierFallback`, warn. The fallback drops the auth requirement too, or the operator would be locked out of a listener they cannot pair against.
4. Port from config/`-port`, default `1990`.
5. `access: local` with a `-listen` that resolves off-loopback: **refuse to start.** The flag picks an address; it cannot change the tier.

CGNAT membership decides **TLS only**. It never establishes identity — see hard constraint 17.

Log the bound address, tier, URL, and whether TLS/auth are on, once at INFO.

## Pairing (the only credential)

There is no password. `homebase pair` mints a **one-time, 10-minute** token and
prints a URL; opening it in a browser exchanges it for a long-lived session
cookie. Minting requires the ability to run a command on the host — local
terminal or ssh, Homebase does not care which — which is exactly the ability
Homebase grants. **Enrollment therefore confers nothing new**, which is the whole
security argument. Do not add an enrollment path that weakens it.

State is `devices.json` beside `config.json`, because the `pair` CLI and the
server are separate processes:

```json
{ "version": 1,
  "pending": [ { "hash": "<sha256 of token>",  "created_at": "…", "expires_at": "…" } ],
  "devices": [ { "id": "…", "name": "<UA>", "hash": "<sha256 of secret>", "created_at": "…" } ] }
```

- **Only hashes on disk.** Plaintext token and session secret exist once, in the CLI's stdout and the browser's cookie jar.
- **Redeem is atomic and single-use**: consume + enroll under one lock and one atomic write, so two browsers racing the same link cannot both win.
- **After redeeming, 302 to `/`** so the token leaves the address bar, history, and any Referer.
- Cookie `homebase_session`: `HttpOnly`, `SameSite=Strict`, `Secure` **iff actually serving TLS** (setting it on plain HTTP locks everyone out), 1-year `MaxAge`.
- `Verify` re-reads `devices.json` whenever its mtime/size changed, so a revoke from the CLI takes effect on the next request. An unchanged file costs one `stat`, not a read. **Do not "optimize" this into an unbounded in-memory cache — that silently breaks revocation.**
- `LastSeen` is in-memory only. Persisting it would add a background writer racing the CLI.
- Corrupt `devices.json` is a startup error, never a silent reset: that would revoke every device without saying so.

Public CLI: `homebase start` `stop` `restart` `status` `pair` `version`.
No-args prints help (it does not start the server). `homebase serve` is the
foreground HTTP process registered by `start` as `<abs-path> serve`; it is
not listed in help. Access and device revoke live in Settings (loopback
peer only). `homebase pair` still exists because minting a token is "can
run a command on this machine".

Switching tiers requires a restart. Rebinding a live listener (and swapping in a
cert) is not worth the rollback complexity for a one-time operation.

### Pairing URLs

`pair` recomputes the bind address itself rather than asking the server, so it
works while the server is down. With a concrete bind address it prints one exact
URL. Under `lan` the listener is `0.0.0.0` and the program **cannot know which
interface the browser will use**, so it prints every routable local IPv4 as a
candidate and says so. Any address that reaches the machine works with the same
`/pair?t=…` path.

## Frontend

- **One** xterm.js instance and **one** WebSocket for the whole page. Switching host or window does not create a second terminal.
- `allowProposedApi: true` is **required** — `addon-unicode11` and `term.unicode.activeVersion` are proposed API and throw without it.
- Host switcher top-left. First entry is `local`, labeled 本机, not editable or deletable.
- Sidebar = the current host's tmux windows. Click → select, `+` → new, pencil → rename, `×` → kill (disabled when only one window remains).
- Refresh the list immediately after every successful control action, plus poll every 3s. Pause polling when `document.visibilityState === "hidden"`.
- `ResizeObserver` on the terminal container, debounce ~32ms, `fit()` + resize message.
- Reconnect: 1s → 2s → 4s → 8s → 16s → 30s, stay at 30s, never stop. Reset on `state=connected`.
- Fonts: `system-ui` and `ui-monospace`. Do not vendor `.woff` / `.woff2`.

## Config schema (v3)

Path: `$XDG_CONFIG_HOME/homebase/config.json` → `~/.config/homebase/config.json`. Flag `-config` overrides. `devices.json` and `tls/` live in the same directory.

Missing file on startup: create the parent directory `0700`, atomically write this default `0600`, continue. Do not seed any host. A fresh install is loopback-only: usable immediately, exposed to nothing.

```json
{
  "version": 3,
  "access": "local",
  "listen_addr": "",
  "listen_port": 1990,
  "tls": { "cert_path": "", "key_path": "" },
  "hosts": []
}
```

`tls` both-empty means Homebase manages a self-signed pair under `<config dir>/tls/`. Set both to supply your own (e.g. from `tailscale cert`).

Populated:

```json
{
  "version": 3,
  "hosts": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "label": "Mac mini",
      "host": "mac-mini",
      "user": "yourname",
      "order": 1
    }
  ]
}
```

- `id` is a server-generated UUID. POST ignores client-supplied ids. The literal id `local` is reserved.
- Write: temp file in the same directory, `fsync`, `rename`. Mutex around read-modify-write.
- Unknown JSON fields: ignore (this is how a v1 file's `windows` array is dropped). Missing `version`: treat as current.
- File exists but corrupt, `version` > 3, or `access` not one of the three: refuse to start; do not overwrite.
- **v2 → v3 migration**: a file with no `access` maps `allow_public_bind: true` → `lan`, otherwise → `private`. v2 bound the Tailscale IP by default, so this preserves the existing listener instead of silently moving an upgraded install onto loopback. Note that upgrading a v2 install therefore *starts requiring pairing* — that is the point, and the startup log says so.

REST (JSON, same origin):

```
GET    /api/health                        {ok:true, listen:"…"}   (no auth)
GET    /api/windows                       {windows:[{index,name,active}]}
POST   /api/windows                       → {index}
PUT    /api/windows/{index}               {name:"…"} or {active:true}
DELETE /api/windows/{index}               last window → 400
GET    /api/settings                      loopback TCP peer only
PUT    /api/settings                      {access, trusted_ranges} → {…, restart_required:true}
GET    /api/devices                       loopback TCP peer only; {devices:[{id,name,created_at}]}
DELETE /api/devices/{id}                  204; unknown id → 404
```

`/api/settings` and `/api/devices` use `requireLoopbackPeer` (the TCP peer
address, not `auth.RequireLoopbackHost`). A paired phone on `private`/`lan`
must not be able to change Access or revoke another device. Device JSON is
a dedicated response struct: never marshal `devices.Device` (that would
leak `hash`). Pending pairing tokens are not listed.

Mutating requests: require the `Origin` header host to match the request host. Missing Origin (curl) is allowed.

Cap: **16** remote hosts. POST past the cap returns 400.

## Threat model

**The local host means anyone who can reach the page can run commands on the
Homebase machine**, with the privileges of the user running the process — the
same as a Terminal window, minus the requirement to be sitting at the machine.
That is not a bug; it is the product. What follows from it:

- The bind address and the device session are the *entire* boundary. Say so in README.
- Assume the LAN is hostile. A passive sniffer that captures the session cookie has a permanent shell, which is why `lan` cannot be plaintext.
- Loopback is exempt from auth *only* because reaching it already implies local shell access — and only while the `Host` header is pinned, since DNS rebinding otherwise lets a remote web page reach it.
- Remote hosts are unaffected: they still require SSH keys (constraint 2). The device session is an additional gate, never a replacement.

## Easy to break

- **`allowProposedApi: false` + unicode11 addon.** Throws at terminal construction; the error surfaces as an unrelated "cannot load list" message.
- **The chained `;` in the attach command.** Unquoted, `/bin/sh` eats it; not its own argv element, tmux folds it into the session name.
- **`-tt` on the control channel.** Allocates a tty for a one-shot command and pollutes stdout with `\r`. Only the PTY channel gets `-tt`.
- **Homebrew tmux PATH.** `ssh host tmux` is a non-login shell; `/opt/homebrew/bin` is missing. The `/bin/sh` PATH prefix is mandatory. The local runner needs the same PATH augmentation under launchd.
- **Fish/csh as login shell.** Any `{ …; }` or `$( )` in the remote string will misparse.
- **`ControlPath` length.** Over 104 bytes on macOS and ssh fails with a confusing error.
- **Go `string(ptyBytes)`.** Invalid UTF-8 in TUI output corrupts the stream. Copy `[]byte`.
- **Killing the last window.** Destroys the session. Refuse server-side.
- **`new-window` without `-c`.** Silently inherits the *server process's* cwd, not the active pane's, so every new window opens in the same wrong directory. Read `#{pane_current_path}` first. A unit test asserting only the argv will not catch this — the live test that opens a real window and checks its cwd will.
- **`display:none` + FitAddon.** Reports 0×0; the first TUI is garbage. Keep layout size.
- **First-time host keys.** `BatchMode=yes` cannot answer `yes/no`. Surface `ssh_hostkey` and tell the operator to `ssh user@host` once **on the Homebase machine**.
- **Caching device lookups without an invalidation check.** Revocation stops working and nothing fails loudly. `Verify` must notice the file changed.
- **`Secure` cookie on a plaintext listener.** The browser silently withholds it and every request looks unpaired. `Secure` must track whether TLS is actually being served, not whether it is desirable.
- **Regenerating the self-signed cert on restart.** Every paired browser's accepted exception dies and the operator gets a fresh scary warning on each device. Reuse until near expiry.
- **Serving the pairing page without redirecting.** Leaving `?t=` in the address bar puts a live credential into history and `Referer`. 302 to `/` immediately.
- **Tailscale CLI on `PATH`.** The App Store build never installs one, so a `PATH`-only probe silently downgrades to loopback and looks like "Tailscale is unsupported". Scan interfaces for `100.64.0.0/10` first.
- **Requiring auth on the `private` loopback fallback.** No Tailscale address means the listener is loopback; demanding a pairing there locks the operator out of a listener nothing else can reach.

## Tests (required as you build)

Run from repo root: `go test ./...`

- `ident`: reject `; rm -rf`, `$(reboot)`, newlines, empty, oversize; window index bounds
- `config`: round-trip, atomic write, missing file writes default (`access: local`), corrupt file error, v1 file loads with `windows` dropped, v2 file maps `allow_public_bind` to a tier, host cap, `local` id rejected
- `listen`: each tier's bind address and its derived `NeedsAuth`/`NeedsTLS`/`NeedsHostCheck`; `-listen` cannot escape `access: local`; CGNAT boundary (`100.127.255.254` in, `100.128.0.1` out); Tailscale parse; private→loopback fallback drops auth
- `devices`: mint/redeem round-trip, single-use, expiry, revoke, **concurrent redeem elects exactly one winner**, only hashes on disk, a second process sees a minted token and a revoke, corrupt file errors
- `api` devices: list/revoke loopback-only, response has no `hash`, unknown id → 404, Settings PUT returns `restart_required`
- `auth`: unpaired and forged cookies rejected, pair→cookie→access, replay rejected, revoked device loses access, `Host` pinning accepts loopback names and rejects rebinding hosts, API denials are JSON not HTML
- `tlsgen`: generated cert covers the requested host plus loopback, key is `0600`, second call reuses the same certificate
- `tmux`: argv construction for local and ssh; `list-windows` output parsing; `can't find session` → empty list; last-window kill refused; `-c` present/absent in attach and new-window argv; live: a new window opens in the active pane's directory, not the server's cwd
- `session`: local dialer start/resize/write/kill leaves no leaked process
- `ws` protocol: resize JSON, binary vs text, ignore unknown type, unknown id closes 4404
- frontend logic if extracted: backoff sequence `1,2,4,8,16,30,30`

Do not require a second physical host for CI. Live ssh is a manual probe, not a unit test.

## What not to do

- Multi-user accounts, OAuth, SSO
- Electron / Tauri / Wails
- More than one tmux session per host
- SFTP, file upload
- Session recording, replay, AI, our own split-pane UI (tmux does splits)
- SQLite
- CDN for xterm.js or fonts
- Binding all interfaces "just for convenience"
- A WebSocket connection-count limiter
- A password, a signup flow, or a "remember me" checkbox
- ACME/Let's Encrypt (no public CA signs `192.168.x.x`)
- Trusting an IP range, a VPN interface name, or a header as proof of identity
- A "skip TLS" or "disable auth" escape hatch for the routable tiers

## Build

```bash
go build -o homebase ./cmd/homebase
go test ./...
```

A local `go build` is for development. It does not ship.

## Git

Commit messages are English (hard constraint 18). One idea per commit; subject line ≤ 72 characters.

## Releases

GitHub Actions (`.github/workflows/release.yml`) is the only publisher. On a push of tag `v*`:

1. Checkout with full history
2. GoReleaser builds `darwin`/`linux` × `amd64`/`arm64` (`CGO_ENABLED=0`)
3. Uploads `homebase_<version>_<os>_<arch>.tar.gz`, `checksums.txt`, and `install.sh`

`{{.Version}}` in GoReleaser is the tag without the `v`, so tag `v0.1.0` produces `homebase_0.1.0_darwin_arm64.tar.gz` and `homebase version` prints `0.1.0`. That name is a contract with `install.sh` — do not rename it.

To ship:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

Do not `goreleaser release` from a development machine. Do not drag files onto a GitHub Release. A tag `0.1.0` (no `v`) will sit there and do nothing.
