// Package tmux owns every tmux argv Homebase issues against the local
// homebase session (control channel: short-lived commands; PTY channel: the
// attach command).
package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
)

// SessionName is the only tmux session Homebase ever touches.
const SessionName = "homebase"

// Errors surfaced by a Runner.
var (
	// ErrNoTmux means the tmux binary was not found on the target.
	ErrNoTmux = errors.New("tmux not found")
	// ErrNoSession means the homebase session does not exist yet. Callers
	// treat this as "empty", not as a failure: the PTY connect creates it.
	ErrNoSession = errors.New("tmux session does not exist")
)

// listFormat puts the machine fields first and the free-form window name
// last, separated by single spaces. Do not use a tab: tmux renders control
// characters in -F output as "_" when the process has no UTF-8 locale (as
// under launchd), which silently turned the whole list into garbage.
//
// window_activity is a unix timestamp of the window's last pane output, and
// window_bell_flag says the program in it rang. Both need zero configuration:
// activity is bookkeeping tmux does whether or not monitor-activity is set,
// and monitor-bell is on by default. That matters, because the obvious
// alternative — window_activity_flag plus "set-option -t homebase
// monitor-activity on" — does not work: monitor-activity is a *window*
// option, so targeting a session sets it on that session's current window
// only and windows created later never inherit it. The session-wide form
// ("set-option -g -w") would leak into the user's other tmux sessions on the
// same server, and Homebase must not reach outside its own session.
const listFormat = "#{window_index} #{window_active} #{window_activity} #{window_bell_flag} #{window_name}"

// AttachArgs is the PTY channel command: attach to the session (creating it
// if needed) and turn off the status bar for that session only. dir sets the
// start directory for a brand-new session; -A ignores it when the session
// already exists, so it is always safe to pass. Empty dir omits -c, leaving
// the directory to whatever the invoking process's cwd is.
//
// The ";" must stay its own argv element. Locally tmux needs it as a separate
// argument; remotely it is POSIX-single-quoted so /bin/sh does not eat it.
func AttachArgs(dir string) []string {
	args := []string{"new-session", "-A", "-s", SessionName}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	return append(args,
		";",
		"set-option", "-t", SessionName, "status", "off",
	)
}

// ListArgs lists the windows of the session, one per line.
func ListArgs() []string {
	return []string{"list-windows", "-t", SessionName, "-F", listFormat}
}

// ClientsArgs lists the tmux clients attached to the session, one per line.
// Anyone attached — a Homebase browser tab or a plain `tmux attach` over ssh
// — counts, because any of them can shrink the shared window via tmux's
// smallest-client resize behavior.
func ClientsArgs() []string {
	return []string{"list-clients", "-t", SessionName}
}

// CurrentPathArgs prints the working directory of the session's currently
// active pane, used to seed a new window's start directory.
func CurrentPathArgs() []string {
	return []string{"display-message", "-p", "-t", SessionName, "#{pane_current_path}"}
}

// NewWindowArgs creates a window and prints its index. dir, if non-empty,
// becomes the new window's start directory via -c.
func NewWindowArgs(dir string) []string {
	args := []string{"new-window", "-t", SessionName}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	return append(args, "-P", "-F", "#{window_index}")
}

// RenameWindowArgs renames one window. tmux clears that window's
// automatic-rename as a side effect, which is what makes the name stick.
func RenameWindowArgs(index int, name string) []string {
	return []string{"rename-window", "-t", target(index), name}
}

// KillWindowArgs kills one window. Callers must refuse to kill the last one:
// that would destroy the session.
func KillWindowArgs(index int) []string {
	return []string{"kill-window", "-t", target(index)}
}

// maxScrollLines bounds one scroll request. tmux clamps at the ends of its
// own history by itself; this only stops a buggy or hostile client from
// making us build an absurd repeat count.
const maxScrollLines = 500

// CopyModeArgs puts the session's active pane into copy mode, which is the
// only place tmux lets anything move through its scrollback.
//
// "-e" makes tmux leave copy mode on its own once the view reaches the
// bottom again, which is what turns "swipe back down" into "return to the
// live shell" with nothing to dismiss. Re-entering while already in copy mode
// keeps the current scroll position, so callers may issue this before every
// scroll and stay stateless.
func CopyModeArgs() []string {
	return []string{"copy-mode", "-e", "-t", SessionName}
}

// ScrollArgs moves the active pane's view: positive lines go back into
// history, negative come forward again. "send-keys -X" outside copy mode
// fails with "not in a mode", so CopyModeArgs has to run first.
func ScrollArgs(lines int) []string {
	cmd := "scroll-up"
	if lines < 0 {
		cmd, lines = "scroll-down", -lines
	}
	if lines > maxScrollLines {
		lines = maxScrollLines
	}
	return []string{"send-keys", "-t", SessionName, "-X", "-N", strconv.Itoa(lines), cmd}
}

// CancelCopyModeArgs leaves copy mode and jumps back to the live output.
func CancelCopyModeArgs() []string {
	return []string{"send-keys", "-t", SessionName, "-X", "cancel"}
}

// InModeArgs prints 1 when the session's active pane is in copy mode.
func InModeArgs() []string {
	return []string{"display-message", "-p", "-t", SessionName, "#{pane_in_mode}"}
}

// SelectWindowArgs makes one window current for the session.
func SelectWindowArgs(index int) []string {
	return []string{"select-window", "-t", target(index)}
}

func target(index int) string {
	return SessionName + ":" + strconv.Itoa(index)
}

// LocalBinary finds tmux for the local host. Under launchd the process has a
// stripped PATH that does not include Homebrew.
//
// Search order is duplicated in install.sh (find_tmux). Change both.
func LocalBinary() (string, error) {
	if p, err := exec.LookPath("tmux"); err == nil {
		return p, nil
	}
	for _, p := range []string{
		"/opt/homebrew/bin/tmux",
		"/usr/local/bin/tmux",
		"/usr/bin/tmux",
		"/opt/local/bin/tmux",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", ErrNoTmux
}

// ExecEnv is the environment for any process Homebase spawns: a PATH that
// includes Homebrew, a TERM, and a HOME even under launchd.
func ExecEnv() []string {
	env := os.Environ()
	env = setEnv(env, "TERM", "xterm-256color")
	env = setEnv(env, "PATH", augmentedPath())
	env = ensureUTF8Locale(env)
	if os.Getenv("HOME") == "" {
		if u, err := user.Current(); err == nil && u.HomeDir != "" {
			env = setEnv(env, "HOME", u.HomeDir)
		} else if h, err := os.UserHomeDir(); err == nil && h != "" {
			env = setEnv(env, "HOME", h)
		}
	}
	return env
}

func augmentedPath() string {
	p := os.Getenv("PATH")
	for _, extra := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"} {
		if !hasPathEntry(p, extra) {
			if p == "" {
				p = extra
			} else {
				p += ":" + extra
			}
		}
	}
	return p
}

func hasPathEntry(path, entry string) bool {
	for _, e := range strings.Split(path, ":") {
		if e == entry {
			return true
		}
	}
	return false
}

func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

// classifyOutput turns tmux's own complaints into the sentinel errors callers
// switch on. It never returns raw PTY bytes; stderr is trimmed to one line.
//
// Only stderr is inspected, and only on a non-zero exit. stdout is data: a
// window is named by the user, and `list-windows -F` prints those names
// straight back. Matching sentinels against stdout meant a window called
// "no server running" turned a perfectly healthy list into ErrNoSession —
// every window vanished from the sidebar and KillWindow started reporting
// "last window" because the list it checked was empty.
func classifyOutput(stdout, stderr []byte, exitErr error) error {
	if exitErr == nil {
		return nil
	}
	lower := strings.ToLower(string(stderr))
	// tmux says "can't find session: homebase", "no server running on …", or
	// "error connecting to … (No such file or directory)" when nothing has
	// ever attached on this machine.
	if strings.Contains(lower, "can't find session") ||
		strings.Contains(lower, "no such session") ||
		strings.Contains(lower, "no server running") ||
		strings.Contains(lower, "error connecting to") {
		return ErrNoSession
	}
	msg := strings.TrimSpace(firstLine(string(stderr)))
	if msg == "" {
		msg = strings.TrimSpace(firstLine(string(stdout)))
	}
	if msg == "" {
		return exitErr
	}
	return fmt.Errorf("tmux: %s", msg)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// ensureUTF8Locale gives tmux and the shell a UTF-8 locale when the parent has
// none. launchd starts agents with no LANG at all; without it tmux mangles
// non-ASCII output and even rewrites control characters in -F formats.
func ensureUTF8Locale(env []string) []string {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := lookupEnv(env, k); v != "" {
			return env
		}
	}
	return setEnv(env, "LANG", "en_US.UTF-8")
}

func lookupEnv(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return kv[len(prefix):]
		}
	}
	return ""
}
