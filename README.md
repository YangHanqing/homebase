# Homebase

*[中文说明](README.zh.md)*

A web terminal for a machine that stays on — a Mac mini, a home server.
Open a browser and you are in a `tmux` session on that machine. Close the
tab, sleep the laptop, drop Wi-Fi: the work is still there.

Homebase does not replace tmux. It attaches to **one** session, always named
`homebase`. The sidebar *is* that session's window list.

**Security is Tailscale's job, not Homebase's.** Homebase speaks plain HTTP.
Other devices should reach it over Tailscale. Do not put it on a raw LAN
unless you understand the risk.

## What you need

**Host** (where `homebase` runs):

| OS | CPU |
| --- | --- |
| macOS | Apple Silicon or Intel |
| Linux | x86_64 or ARM |

Windows cannot be a host. A Windows PC, iPad, or phone is only a browser.

Also:

1. **tmux** on the host (required)
2. **Tailscale** on the host and on any other device you connect from

## 1. Install tmux

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

## 2. Install Homebase

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
```

This only puts the binary in `~/.local/bin`. It does not start anything.

## 3. Start

```bash
homebase start
```

On **this machine**, open [http://127.0.0.1:1990](http://127.0.0.1:1990).

By default Homebase is reachable on `127.0.0.1` and on your Tailscale
address — not on `192.168.x.x`.

## Another device (Tailscale)

1. On the host, open Settings (the gear; only shown at `127.0.0.1`)
2. Access should be **Trusted range**. Save, then:

```bash
homebase restart
homebase pair
```

3. On the other device, while it is on Tailscale, open the printed URL.

## Update

Run the install script again, then restart the service:

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
homebase restart
```

Pin a version with `HOMEBASE_VERSION=v0.1.1` in front of `curl`.

## Without Tailscale: the local network

This is **not** the main path. Settings → Access → **Local network (risk)**,
save (you will be asked to confirm), then `homebase restart` and
`homebase pair`. Do not do this on hotel or office Wi-Fi.

## Commands

| Command | What it does |
| --- | --- |
| `homebase start` | Install the user service and run it |
| `homebase stop` | Stop the service. Config and tmux stay |
| `homebase restart` | Stop, then start. Run this after changing Access |
| `homebase status` | Running?, URL, tmux, version |
| `homebase pair` | One-time login URL (10 minutes, single use) |
| `homebase version` | Build version |

One instance per machine. If it is already running, `start` prints the URL
and does not launch a second copy. If port 1990 is taken by something else,
start tries 1991, 1992, … and saves the port it used.

## From source

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```
