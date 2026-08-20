package tmux

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// ErrLastWindow is returned instead of killing the only remaining window,
// which would take the whole session down with it.
var ErrLastWindow = errors.New("refusing to kill the last window")

// Window is one tmux window inside the homebase session.
type Window struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// Runner executes one short-lived tmux command and returns its stdout.
// It is the control channel and must never touch the PTY.
type Runner interface {
	Run(ctx context.Context, args []string) ([]byte, error)
}

// LocalRunner execs tmux on the machine Homebase itself runs on. No ssh.
type LocalRunner struct {
	// Bin overrides tmux discovery in tests only.
	Bin string
}

// Run implements Runner.
func (r LocalRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	bin := r.Bin
	if bin == "" {
		found, err := LocalBinary()
		if err != nil {
			return nil, err
		}
		bin = found
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = ExecEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if cerr := classifyOutput(stdout.Bytes(), stderr.Bytes(), err); cerr != nil {
		return nil, cerr
	}
	return stdout.Bytes(), nil
}

// Client is the window-management API the REST layer talks to.
type Client struct {
	R Runner
}

// NewClient builds the local control-channel client.
func NewClient() Client {
	return Client{R: LocalRunner{}}
}

// ListWindows returns the session's windows. A session that does not exist
// yet is an empty list, not an error: the PTY connect will create it.
func (c Client) ListWindows(ctx context.Context) ([]Window, error) {
	out, err := c.R.Run(ctx, ListArgs())
	if errors.Is(err, ErrNoSession) {
		return []Window{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseWindows(out), nil
}

func parseWindows(out []byte) []Window {
	windows := []Window{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		// index, active, then the name, which may itself contain spaces.
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		windows = append(windows, Window{
			Index:  idx,
			Active: parts[1] == "1",
			Name:   parts[2],
		})
	}
	return windows
}

// NewWindow creates a window and returns its index. It inherits the working
// directory of the currently active window, if that can be determined.
func (c Client) NewWindow(ctx context.Context) (int, error) {
	out, err := c.R.Run(ctx, NewWindowArgs(c.currentPath(ctx)))
	if err != nil {
		return 0, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, errors.New("tmux did not print a window index")
	}
	return idx, nil
}

// currentPath is best-effort: a failure (no session yet, tmux hiccup) just
// means the new window falls back to the invoking process's own cwd.
func (c Client) currentPath(ctx context.Context) string {
	out, err := c.R.Run(ctx, CurrentPathArgs())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RenameWindow sets a window's name. tmux turns off automatic-rename for that
// window as a side effect, so the name sticks.
func (c Client) RenameWindow(ctx context.Context, index int, name string) error {
	_, err := c.R.Run(ctx, RenameWindowArgs(index, name))
	return err
}

// KillWindow removes a window. It refuses the last one, because tmux would
// destroy the session along with it — see AGENT.md hard constraint 3.
func (c Client) KillWindow(ctx context.Context, index int) error {
	windows, err := c.ListWindows(ctx)
	if err != nil {
		return err
	}
	if len(windows) <= 1 {
		return ErrLastWindow
	}
	_, err = c.R.Run(ctx, KillWindowArgs(index))
	return err
}

// SelectWindow makes a window current for the session, which is what the
// attached PTY redraws.
func (c Client) SelectWindow(ctx context.Context, index int) error {
	_, err := c.R.Run(ctx, SelectWindowArgs(index))
	return err
}
