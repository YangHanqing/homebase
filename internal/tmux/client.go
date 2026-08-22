package tmux

import (
	"bytes"
	"context"
	"errors"
	"os"
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

// ClientCount returns how many tmux clients are attached to the session. A
// missing session means zero, not an error: the PTY connect will create it.
func (c Client) ClientCount(ctx context.Context) (int, error) {
	out, err := c.R.Run(ctx, ClientsArgs())
	if errors.Is(err, ErrNoSession) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}

// NewWindow creates a window and returns its index. dirMode selects the
// start directory: "home" always starts in $HOME (not in whatever cwd the
// homebase process happens to have — often "/" under launchd); anything
// else, including the default "same", copies the currently active pane's
// directory, like a normal terminal's new-tab.
func (c Client) NewWindow(ctx context.Context, dirMode string) (int, error) {
	out, err := c.R.Run(ctx, NewWindowArgs(c.startDir(ctx, dirMode)))
	if err != nil {
		return 0, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, errors.New("tmux did not print a window index")
	}
	return idx, nil
}

// startDir resolves the directory a new window should open in. A missing
// session, or any other lookup failure, falls back to $HOME rather than
// failing window creation over it.
func (c Client) startDir(ctx context.Context, dirMode string) string {
	home, _ := os.UserHomeDir()
	if dirMode == "home" {
		return home
	}
	out, err := c.R.Run(ctx, CurrentPathArgs())
	if err != nil {
		return home
	}
	if dir := strings.TrimSpace(string(out)); dir != "" {
		return dir
	}
	return home
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

// Scroll moves the active pane's view through tmux's history and reports
// whether the pane is still in copy mode afterwards.
//
// This has to go through tmux because tmux, not the browser, owns the
// scrollback. Homebase's xterm is attached to a full-screen tmux client,
// which lives on the alternate screen, so the browser never accumulates any
// scrollback of its own -- there is literally nothing there for a swipe to
// move. lines == 0 means "leave copy mode".
func (c Client) Scroll(ctx context.Context, lines int) (bool, error) {
	if lines == 0 {
		// Cancelling when not in copy mode is "not in a mode", which is the
		// desired end state, not a failure.
		_, _ = c.R.Run(ctx, CancelCopyModeArgs())
		return false, nil
	}
	if _, err := c.R.Run(ctx, CopyModeArgs()); err != nil {
		return false, err
	}
	if _, err := c.R.Run(ctx, ScrollArgs(lines)); err != nil {
		return false, err
	}
	out, err := c.R.Run(ctx, InModeArgs())
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "1", nil
}

// SelectWindow makes a window current for the session, which is what the
// attached PTY redraws.
func (c Client) SelectWindow(ctx context.Context, index int) error {
	_, err := c.R.Run(ctx, SelectWindowArgs(index))
	return err
}
