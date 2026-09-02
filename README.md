# Homebase

*[中文说明](README.zh.md)*

**Take over the terminal on your always-on machine from a browser — on your
phone, on a laptop, anywhere.**

![Homebase web terminal screenshot](.github/images/homebase-ui.png)

## What this is

You have a machine that stays on: a Mac mini under the desk, a MacBook that
never really shuts, a Linux box in a closet. You run long-lived things on it —
Claude Code, Grok Build, a build, a training run, an ordinary shell.

Homebase is a single Go binary that runs on that machine and puts its terminal
in a browser. Open a URL on your phone and you are in the *same* session that
was running when you walked away: same scrollback, same running process, same
half-typed command. Close the tab, sleep the laptop, lose Wi-Fi in a tunnel —
nothing is interrupted, because nothing about the session lives in the browser.

No SSH client to install on the phone. No port forwarding. Nothing exposed to
the public internet.

**If you already know tmux and Tailscale**, the whole thing is: Homebase is a
web frontend for one fixed tmux session on the local machine, reached over your
tailnet, with device pairing as the only credential. Skip to
[Install and start](#install-and-start).

**If you don't**, read the next section — it is the only background you need.

## The two things it stands on

Homebase is small because it does not solve these two problems itself. It
borrows two well-established tools. You do not need to learn them deeply, but
you should know what each one is for.

### tmux — this is what keeps your work alive

Normally, a terminal session belongs to the window it opened in. Close the
window, lose the SSH connection, and everything running in it dies. That is why
a dropped connection halfway through a long job is so annoying.

**tmux** (a "terminal multiplexer") breaks that link. It runs your shell in a
session that lives in the background on the machine itself. Your terminal
window merely *attaches* to that session and draws it. Detach — deliberately,
or because your Wi-Fi dropped — and the session keeps running exactly as it
was. Attach again from anywhere and you are back in it, mid-scroll.

That is the entire property Homebase is built on. The browser is just another
thing that attaches. **Homebase never holds your session state; tmux does.**
This is why closing the tab is safe.

You do not need to learn tmux's keyboard commands to use Homebase. The sidebar
does the common things for you:

- Each row in the sidebar is a tmux **window** — think of it as a tab. Click to
  switch, `+` to open a new one, and you can rename or close them.
- The scrollback you see is tmux's, not the browser's, which is why it survives
  a reconnect.

Everything you already know still works — tmux's own key bindings (splits,
copy mode) are all there if you want them, because what you are looking at is a
real tmux client.

Install it before Homebase — without tmux, Homebase does not run:

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

### Tailscale — this is what lets you reach the machine

Your home machine sits behind a router. From outside, it has no address anyone
can dial. The traditional fixes are all bad ones: forward a port and put an SSH
server on the public internet, or rent a jump host, or run a tunnel service.

**Tailscale** is a VPN that requires no configuration. Install it on the host
and on your phone, sign both into the same account, and each device gets a
stable private address in the `100.x.y.z` range that works from anywhere —
coffee shop, cellular, another country. Traffic between them is encrypted
end-to-end and never touches a public port. There is no router setup, no
firewall rule, no static IP. The free tier covers a personal set of devices.

Homebase's whole address policy is built on this. By default it binds *only*
your loopback address and your Tailscale address, and it will refuse to bind a
public one. So:

- **Homebase itself speaks plain HTTP and has no TLS.** That is deliberate.
  Encryption is Tailscale's job, and doing it twice buys nothing.
- Which is also why, on a plain LAN without Tailscale, everything you type is
  visible to anyone else on that network. Read
  [Security model](#security-model) before you go that route.

Get it at [tailscale.com/download](https://tailscale.com/download), on the host
*and* on every device you want to connect from.

> **Already using WireGuard, ZeroTier, or another overlay?** That works too.
> Homebase's default trusted range is Tailscale's `100.64.0.0/10`; point it at
> your own range in Settings.

## Install and start

On the always-on machine:

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
```

This downloads a signed release binary (verified against the release
checksums), puts it at `~/.local/bin/homebase`, and installs nothing else.
Running the same command later is how you **upgrade** — it atomically replaces
the binary in place. After an upgrade, run `homebase restart` for the new
version to take effect.

Then:

```bash
homebase start
```

That registers Homebase as a background service that survives reboots — a
launchd agent on macOS, a systemd user unit on Linux — and prints the URL.

Open that URL **on the host machine itself** (by default
[http://127.0.0.1:1990](http://127.0.0.1:1990)). You should get a terminal
immediately, with no login. That is the loopback address, and reaching it
already means you have local access to this machine.

You are done, for this machine. The rest is about reaching it from elsewhere.

## Connecting from your phone or another computer

Do this once, on the host.

**1. Make sure Tailscale is running on both devices**, signed into the same
account. On the host, `tailscale ip -4` prints its `100.x.y.z` address.

**2. Pair the device.** In the terminal on the host:

```bash
homebase pair
```

It prints a one-time link:

```
Open this link on the device you want to pair:

    http://100.101.102.103:1990/pair?t=9f2a6c1e4b7d8a3f5c0e1b2d3a4f5c6e

Valid until 21:45:10 (10 minutes), single use only.
```

**3. Open that link on the other device's browser.** It trades the token for a
long-lived cookie, and from then on that device just goes to
`http://100.101.102.103:1990` and gets the terminal. Bookmark it, or add it to
your phone's home screen.

The link is valid for 10 minutes and works exactly once. Pair each device once.

Why a link and not a password? Because you had to be able to run a command on
the machine to get it — and running commands on the machine is precisely the
thing Homebase hands out. Pairing therefore grants nothing you didn't already
have, which is why there is no account to create and no password to forget.

You can see and revoke paired devices in Settings, which is visible only when
you are browsing from `127.0.0.1` on the host.

### Reaching it over a plain LAN instead

If you would rather not use Tailscale and only want other devices on your own
home network to connect: on the host, open Settings → Access and choose **All
local networks**, then pair as above. Saving restarts the service for you. From
the command line the same thing is:

```bash
homebase access lan && homebase restart
```

Understand what you are choosing: this is unencrypted HTTP on your LAN. Anyone
else on that network can read everything you type and everything the terminal
prints, and can steal the session cookie off the wire. On your own home Wi-Fi
that may be an acceptable trade. On café or office Wi-Fi it is not. Homebase
shows you a red confirmation for exactly this reason.

Homebase will never bind a public address, on any setting. There is no
configuration, and no command-line flag, that puts it on the open internet.

### macOS: grant Full Disk Access before you need it

If the host is a Mac, do this now, while you are sitting at it.

Add the **Homebase binary** to System Settings → Privacy & Security → Full Disk
Access. The installer puts it at `~/.local/bin/homebase` — that expands to
`/Users/<your-username>/.local/bin/homebase`, and `install.sh` prints the
absolute path when it finishes. If you set `HOMEBASE_INSTALL_DIR`, use that
path.

macOS will never prompt for this permission, and Homebase cannot request it.
Without it, the first time you need to touch Documents, Desktop, or Downloads
while you are away — phone only, no screen sharing — macOS silently blocks it,
and there is no remote "Allow" button to press.

`~/.local` is hidden, so Finder will not show it in a normal browse:

1. Click **+**.
2. In Finder, **Go → Go to Folder…** (⇧⌘G).
3. Paste `~/.local/bin` and press Return.
4. Select the file named `homebase` — the binary, not the folder.

Dragging that file onto the list works too. Granting the *folder*, or a
different copy of the binary, does not count. Upgrades reuse the same path, so
you normally only do this once.

## Commands

| Command | What it does |
| --- | --- |
| `homebase start` | Register a user-level background service and start it |
| `homebase stop` | Stop the service. Config file and tmux session are untouched |
| `homebase restart` | Stop, then start — run this after upgrading the binary |
| `homebase status` | Whether it's running, when it started, bind address, URL, tmux path, version, pairing state, paired device count |
| `homebase pair` | Print a one-time login link: 10 minutes, single use |
| `homebase version` | Print the build version |

Running `homebase` with no arguments prints help and starts nothing.

One machine needs one copy. Running `start` again while it is already running
does not launch a second instance — it prints the address already in use. If
port 1990 is taken it tries 1991, 1992, … and remembers the port it settled on.

There is also `homebase serve`, the foreground process that `start` registers
with the system service manager. You should not need it except for debugging.

## Security model

Homebase hands a browser a shell. Read this before pointing it at a network.

**Homebase speaks plain HTTP. It has no TLS, and no setting turns TLS on.**
Transport encryption belongs to the overlay network — Tailscale, WireGuard, or
whatever you already trust — which is why the default access tier binds only
loopback and your Tailscale range. On an ordinary LAN, anyone else on that
network can read everything you type and everything the terminal prints, and
can lift the session cookie or a pairing link straight off the wire. That is
what the red confirmation on Settings → Access → "All local networks" is
warning you about.

**Access (private / LAN) is the one security-relevant setting.** It decides
which address the service binds, and the authentication requirement — whether
pairing is needed — is *derived* from that address rather than configured
separately. There is no combination that means "bind a routable address, but
skip credentials." Homebase refuses by design to bind any publicly routable
address or `0.0.0.0`, and the `-listen` flag cannot override it, so even a
misconfiguration cannot put an unprotected port on the public internet.

**Loopback (`127.0.0.1`) is reachable without pairing**; every other address
requires a paired device. Pairing grants no capability beyond what was already
there, since running `homebase pair` requires being able to run a command on
the machine — exactly the capability Homebase hands out.

**This assumes a host you do not share.** Because loopback needs no credential,
any other local account on the machine can reach `http://127.0.0.1:1990` and
get a shell as the user running Homebase. On a single-user Mac or a personal
Linux box, that is the same shell those accounts could have had anyway; on a
shared multi-user host it is a privilege escalation. Do not run Homebase on a
machine whose other local accounts you would not hand a terminal to.

**Credentials are stored as hashes only**, in a local `devices.json`. The
plaintext one-time token and the session key each appear exactly once — on the
CLI's stdout and in the browser's cookie. Revoke a device from the Settings
page, visible only over a loopback connection.

To report a vulnerability, see [SECURITY.md](SECURITY.md). Please do not open a
public issue for one.

## What Homebase is not

Deliberate omissions, so you can tell quickly whether this is the tool you
want:

- **Not multi-host.** One binary drives the machine it runs on. No ssh, no host
  list, no jumping between servers.
- **Not a TLS server.** See above. The overlay network does that.
- **Not multi-user.** No accounts, no passwords, no roles. One person, their
  devices.
- **Not a terminal emulator with its own ideas.** Splits, layouts, and copy
  mode are tmux's, and they work as they always did.
- **Not a file manager.** No upload, no SFTP, no session recording.

If you need SSH into arbitrary hosts with per-user accounts, you want a
different tool.

## From source

Requires Go 1.24+ and tmux. There is no frontend build step.

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```

## Contributing

Bug reports and pull requests are welcome — see
[CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and, more
importantly, for what is out of scope. Security issues go through
[SECURITY.md](SECURITY.md), never a public issue.

## License

[MIT](LICENSE).
