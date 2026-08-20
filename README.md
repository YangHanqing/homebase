# Homebase

*[中文说明](README.zh.md)*

A self-hosted web terminal. Run it on a machine that's always on (a Mac mini,
a home server). Open a browser. You're in a `tmux` session on that machine.

Close the tab, sleep the laptop, drop Wi-Fi — the work is still in tmux.
Homebase is just the bridge.

The UI's window list *is* that session's tmux windows. Create, rename, close,
switch, all from the browser. You never name the session, and you never have
to press `C-b`.

## Install

macOS (Apple Silicon or Intel) and Linux. The only extra dependency is
**tmux**. Windows, iPad, and phones are browsers, not hosts.

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
homebase start
```

Then, on this machine, open **http://127.0.0.1:1990**.

That's the whole path if you only need this computer. No pairing, no password.

The installer puts the binary in `~/.local/bin` and leaves it there.
`homebase start` registers a user service (launchd on macOS, systemd on
Linux) so it comes back after a reboot, then returns you to the prompt.

Pin or roll back a version with `HOMEBASE_VERSION=v0.1.0` in front of the
`curl` line. Upgrading is the same command again; if the service is already
running, follow it with `homebase restart`.

## Another device (phone, laptop)

There is no password. Pairing a device means: you can already run a command
on this machine, so you mint a one-time link and open it over there.

1. On this machine, open [http://127.0.0.1:1990/settings.html](http://127.0.0.1:1990/settings.html)
2. Set **Access** to **Trusted range** (your Tailscale / WireGuard network),
   or **Local network** if you understand the risk below
3. Save, then in a terminal on this machine:

```bash
homebase restart
homebase pair
```

4. Open the printed URL on the other device. It stays signed in.

Repeat `homebase pair` for each new device. Revoke one from Settings (same
page, only reachable from this machine).

## Commands

| Command | What it does |
| --- | --- |
| `homebase start` | Write the user service and run it |
| `homebase stop` | Stop the service. Config and tmux stay |
| `homebase restart` | `stop` then `start`. Use this after changing Access |
| `homebase status` | Running?, bind address, URL, tmux, version |
| `homebase pair` | One-time login URL, valid 10 minutes |
| `homebase version` | Build version |

## ⚠️ Read this before leaving this machine

Homebase speaks **plain HTTP, not HTTPS**. Whoever can open the page can run
commands on this machine, as the user running Homebase — the same as sitting
at its keyboard.

Whether that's a problem depends on where you bind it:

| Where | Safe? |
| --- | --- |
| `127.0.0.1` (default) | Yes. Traffic never leaves the machine. |
| A **trusted range** you configured (Tailscale, WireGuard, …) | Yes *if* that range really is already encrypted. Homebase cannot verify this — it's your assertion, made in Settings. |
| Any other network (LAN, hotel, office Wi-Fi) | **No.** Someone on that network can intercept the session and take the machine. |

`access` is the one knob. It lives in Settings, not in the CLI:

| Access | Binds | Pairing |
| --- | --- | --- |
| Local only (default) | `127.0.0.1` | No |
| Trusted range | first address inside your trusted ranges, else loopback | Yes |
| Local network | `0.0.0.0` | Yes |

Local only needs no login because reaching `127.0.0.1` already means you can
open a terminal here. (The `Host` header is pinned so a random web page
cannot DNS-rebind its way in.)

Anything else requires pairing — but pairing is authentication, not
encryption. It stops a stranger from using the UI; it does **not** stop
someone on the same untrusted network from reading the session cookie off
the wire. Only put Homebase on a network you fully control.

## Settings and config

Settings (`/settings.html`) is reachable only from `127.0.0.1`, even when
Access is Trusted range or Local network. Changing Access or trusted ranges,
and revoking devices, always requires being on the machine itself.

After you save Access, run `homebase restart` in a terminal on this machine.
The process will not restart itself from the browser.

Default config: `~/.config/homebase/config.json` (override with `-config` on
`start` / `serve` / `status` / `pair`). First run writes a safe default:
`access: local`, directory `0700`, file `0600`.

```json
{
  "version": 4,
  "access": "local",
  "listen_addr": "",
  "listen_port": 1990,
  "trusted_ranges": ["100.64.0.0/10"]
}
```

`trusted_ranges` is a list of CIDRs you assert are already encrypted.
The default is Tailscale's CGNAT range. Replace it if you use something
else. It is not a cryptographic boundary — it only picks the `private`
bind address and which warning Settings shows.

## From source

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```

`start` registers whatever binary you just ran; it does not copy it into
`~/.local/bin`. Re-vendor xterm.js (rarely needed): `./scripts/vendor-xterm.sh`.

Foreground, no service manager (debugging): `./homebase serve`.

## Security summary

- Always plain HTTP. Encryption, if any, comes from the network you put
  Homebase on, not from Homebase itself.
- No password. Pairing requires already being able to run a command here, so
  it grants no new capability.
- Pairing tokens expire in 10 minutes and are single-use; only a SHA-256
  hash is stored.
- Off loopback, a paired device is required. `trusted_ranges` is a label for
  the Settings warning, not a proof of identity.
- Settings and device revoke are loopback-only.
- Never `kill-server` / `kill-session`. The last tmux window in the session
  refuses to close.
- Don't paste keys, password hashes, or raw terminal output into logs or issues.
