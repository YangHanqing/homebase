# Homebase

*[中文说明](README.zh.md)*

## What this is

If you have a machine that stays on 24/7 — a Mac mini or a MacBook — and you
like running Claude Code, Grok Build, or similar CLI agents in a TUI,
Homebase lets you take over that terminal session from your phone or another
computer, through a browser. No extra SSH client, and the machine never has
to be exposed to the public internet.

Homebase is a single Go binary that runs on that always-on machine. It
connects the browser to **one fixed tmux session** on that machine (always
named `homebase`); the sidebar is that session's tmux window list. All
session state lives in tmux — Homebase is only a PTY forwarding channel:
closing the tab, sleeping the laptop, or a network drop never interrupts
whatever is running.

![Homebase web terminal screenshot](.github/images/homebase-ui.png)

## Prerequisites

**tmux (required).** Homebase does not manage session state itself — tmux is
what actually makes "close the lid, switch devices, keep going" true. The
browser is just attaching to a tmux session that keeps running; on
reconnect, tmux redraws the screen from its own scrollback. Without tmux,
Homebase does not work.

```bash
# macOS (Homebrew)
brew install tmux

# Debian / Ubuntu
sudo apt install tmux

# Fedora
sudo dnf install tmux

# Arch
sudo pacman -S tmux
```

**Tailscale (strongly recommended).** Homebase itself speaks plain HTTP —
it does not provide encryption on its own. It relies on Tailscale to build a
private network between your devices, so connecting from your phone back to
the machine at home never requires exposing a port to the public internet.
Homebase's bind-address policy is built around this: by default it only
binds the loopback address and your Tailscale range, never a public or
unspecified address. Both the host and any device connecting to it need
Tailscale installed and signed into the same tailnet.

## Install and start

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
```

Running this the first time installs Homebase; running it again later is
how you upgrade — it downloads the latest binary and atomically replaces
`~/.local/bin/homebase`. If the service is already running, upgrading
requires a manual `homebase restart` for the new version to take effect.

Then start the service:

```bash
homebase start
```

On **this machine**, open the address printed on the command line (default:
[http://127.0.0.1:1990](http://127.0.0.1:1990)). By default only `127.0.0.1`
and this machine's Tailscale address are reachable — a LAN address like
`192.168.x.x` is not, by default. If you want other devices on your local
network to reach it directly, go to Settings → Access and pick "All local
networks"; saving restarts the service automatically. The same thing from
the command line: `homebase access lan && homebase restart`.

To connect from another device (phone, another computer):

1. On the host, open Settings (only visible when browsing `127.0.0.1`) and
   set Access to Trusted range; saving restarts the service automatically.
2. Run `homebase pair` and open the printed one-time link on another device
   that is already on the same tailnet. The first time you pair, you need to
   already be logged into this machine some ordinary way — a local terminal
   or SSH — before you can run this command: being able to run a command on
   the machine at all is exactly what proves you're allowed to.

`homebase pair` prints something like this:

```
Open this link on the device you want to pair:

    http://100.101.102.103:1990/pair?t=9f2a6c1e4b7d8a3f5c0e1b2d3a4f5c6e

Valid until 21:45:10 (10 minutes), single use only.
```

Opening that link in the other device's browser exchanges the token for a
long-lived login cookie.

### macOS: grant Full Disk Access ahead of time

If the host is a Mac, add `~/.local/bin/homebase` to System Settings →
Privacy & Security → Full Disk Access now. macOS requires a one-time
interactive click in the GUI to authorize access to folders like Documents,
Desktop, and Downloads. If you don't set this up in advance, the first time
Homebase needs to read one of those folders while you're away — operating
only from your phone or a remote terminal, with no screen-sharing session
open on this machine — macOS will silently block it, and there is no way to
click "Allow" remotely.

## Commands

| Command | What it does |
| --- | --- |
| `homebase start` | Registers a user-level background service (a launchd agent on macOS, a systemd user unit on Linux) and starts it |
| `homebase stop` | Stops the service. Config file and tmux session are untouched |
| `homebase restart` | Stop, then start — use this after upgrading the binary |
| `homebase status` | Prints whether it's running, the tmux path, version, bind address, URL, whether pairing is needed, and the number of paired devices |
| `homebase pair` | Generates a one-time login link, valid for 10 minutes, single use |
| `homebase version` | Prints the running binary's build version |

Running `homebase` with no arguments only prints help and starts nothing.
There is also `homebase serve`, the foreground process that `start`
registers with the system service manager internally; you shouldn't need to
call it directly — it's for debugging.

One machine only needs one running copy of Homebase. Running `start` again
while it's already running does not launch a second instance — it prints
the address already in use. If port 1990 is taken, it tries 1991, 1992, …
and remembers whichever port it actually used.

## Security model

Homebase hands a browser a shell. Read this section before pointing it at a
network.

**Homebase speaks plain HTTP. It has no TLS, and no setting turns TLS on.**
Transport encryption is Tailscale's job (or WireGuard's, or any overlay you
already trust) — which is why the default access tier binds only loopback and
your Tailscale range. On an ordinary LAN, anyone else on that network can read
everything you type and everything the terminal prints, and can lift the
session cookie or a pairing link straight off the wire. That is what the red
confirmation on Settings → Access → "All local networks" is warning you about.

Access (private / LAN) is the one security-relevant setting. It decides the
address the service binds to, and the authentication requirement (whether
pairing is needed) is **derived** from that address rather than configured
separately — there is no combination that means "bind a routable address, but
skip credentials". Homebase refuses by design to bind any publicly routable
address or `0.0.0.0`, and the `-listen` flag cannot override that, so even a
misconfiguration cannot put an unprotected port on the public internet.

The loopback address (`127.0.0.1`) is reachable without pairing; any other
device must pair first (see "Install and start" above). Pairing does not grant
any capability beyond what's already there — running `homebase pair` requires
already being able to run a command on the machine, which is exactly the
capability Homebase hands out.

**This assumes the host is a machine you do not share.** Because loopback needs
no credential, any other local account on the host can reach
`http://127.0.0.1:1990` and get a shell as the user running Homebase. On a
single-user Mac or a personal Linux box that is the same shell those accounts
could have had anyway; on a shared multi-user host it is a privilege
escalation. Do not run Homebase on a machine whose other local accounts you
would not hand a terminal to.

Device session credentials are stored only as hashes in the local
`devices.json`. The plaintext one-time token and session key each appear
exactly once — on the CLI's stdout, and in the browser's cookie,
respectively. You can revoke a device's access from the Settings page
(visible only over a loopback connection).

## From source

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```
