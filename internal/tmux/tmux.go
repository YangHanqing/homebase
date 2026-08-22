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

// listFormat puts the two machine fields first and the free-form window name
// last, separated by single spaces. Do not use a tab: tmux renders control
// characters in -F output as "_" when the process has no UTF-8 locale (as
// under launchd), which silently turned the whole list into garbage.
const listFormat = "#{window_index} #{window_active} #{window_name}"

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

// classifyOutput turns tmux/ssh chatter into the sentinel errors callers
// switch on. It never returns raw PTY bytes; stderr is trimmed to one line.
func classifyOutput(stdout, stderr []byte, exitErr error) error {
	lower := strings.ToLower(string(stdout) + string(stderr))
	// tmux says "can't find session: homebase", or "no server running on …"
	// when nothing has ever attached on this machine.
	if strings.Contains(lower, "can't find session") ||
		strings.Contains(lower, "no such session") ||
		strings.Contains(lower, "no server running") {
		return ErrNoSession
	}
	if exitErr == nil {
		return nil
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
