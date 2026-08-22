# Security policy

Homebase attaches a browser to a shell, so a bug in the auth gate, the bind
address policy, or the pairing flow is a remote code execution bug. Please
report those privately.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting:
**[Security → Report a vulnerability](https://github.com/yanghanqing/homebase/security/advisories/new)**.
Please do not open a public issue for anything in the "in scope" list below.

Include the version (`homebase version`), the OS, the Access tier, and enough
detail to reproduce. Expect a first response within a week.

## Supported versions

Only the latest release. Homebase is a single binary; the fix ships as a new
tag and `install.sh` upgrades in place.

## In scope

- Reaching any route without a paired device from a non-loopback address.
- Getting Homebase to bind a publicly routable or unspecified address.
- Redeeming a pairing token more than once, after it expired, or without it.
- Escaping the fixed `homebase` tmux session, or injecting a shell command
  through a window name, index, or any other API field.
- Cross-site attacks that reach the WebSocket or a mutating route from another
  origin (CSRF, DNS rebinding against the unauthenticated loopback listener).
- Leaking a pairing token, session secret, or PTY contents into a log file.

## Out of scope

These are known, documented properties of the design, not vulnerabilities:

- **No TLS.** Homebase always speaks plain HTTP. Anyone who can see traffic on
  the network you exposed it to can read the session and steal the cookie.
  Transport encryption is Tailscale's (or WireGuard's) job — see
  [README.md](README.md#security-model).
- **Loopback needs no credential.** Any local account on the host can get a
  shell as the user running Homebase. Homebase assumes a machine you do not
  share; the same README section spells this out.
- Anything that requires already being able to run commands on the host, since
  that is exactly the capability Homebase hands out.
