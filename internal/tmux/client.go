package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ErrLastWindow is returned instead of killing the only remaining window,
// which would take the whole session down with it.
var ErrLastWindow = errors.New("refusing to kill the last window")

// Window is one tmux window inside the homebase session.
//
// Activity is tmux's own #{window_activity}: the unix time of the last output
// in that window, on the *server's* clock. It is reported raw rather than as
// an age, because a browser's clock is its own; the REST layer sends the
// server's "now" beside the list so the frontend can subtract two readings of
// the same clock. It also updates for background windows, which is the whole
// point — that is how the sidebar can tell which window an agent is working
// in without attaching to it.
type Window struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Activity int64  `json:"activity"`
	Bell     bool   `json:"bell"`
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
	// Session is the tmux session this client operates on. Empty means the
	// legacy singleton session (SessionName), so a zero-value Client keeps
	// working exactly as it did before per-project sessions existed.
	Session string
}

// NewClient builds the local control-channel client for the legacy singleton
// session.
func NewClient() Client {
	return Client{R: LocalRunner{}}
}

// NewClientFor builds a local control-channel client for one tmux session.
func NewClientFor(session string) Client {
	return Client{R: LocalRunner{}, Session: session}
}

func (c Client) session() string {
	if c.Session == "" {
		return SessionName
	}
	return c.Session
}

// ListWindows returns the session's windows. A session that does not exist
// yet is an empty list, not an error: the PTY connect will create it.
func (c Client) ListWindows(ctx context.Context) ([]Window, error) {
	out, err := c.R.Run(ctx, ListArgs(c.session()))
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
		// index, active, activity, bell, then the name, which may itself
		// contain spaces — so it has to be the last field and the split has
		// to stop counting at it.
		parts := strings.SplitN(line, " ", 5)
		if len(parts) < 5 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		// An unparseable timestamp is a missing signal, not a missing
		// window: fall back to zero, which the frontend reads as "quiet",
		// rather than dropping a window the user can still click.
		activity, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			activity = 0
		}
		windows = append(windows, Window{
			Index:    idx,
			Active:   parts[1] == "1",
			Activity: activity,
			Bell:     parts[3] == "1",
			Name:     parts[4],
		})
	}
	return windows
}

// ClientCount returns how many tmux clients are attached to the session. A
// missing session means zero, not an error: the PTY connect will create it.
func (c Client) ClientCount(ctx context.Context) (int, error) {
	out, err := c.R.Run(ctx, ClientsArgs(c.session()))
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
	dir := c.startDir(ctx, dirMode)
	out, err := c.R.Run(ctx, NewWindowArgs(c.session(), dir))
	if err != nil {
		if !c.sessionMissing(err) {
			return 0, err
		}
		// new-window only works on a session that already exists -- unlike
		// the PTY channel's "new-session -A", which is attach-if-exists and
		// therefore idempotent. A per-project session's first window can be
		// requested here before its terminal has ever been opened, so bring
		// the session into being: its initial window IS the requested one.
		out, err = c.R.Run(ctx, NewSessionDetachedArgs(c.session(), dir))
		if err != nil && strings.Contains(err.Error(), "duplicate session") {
			// Lost a race with a concurrent PTY attach creating the same
			// session (the browser opens both at once when the sidebar's
			// "+" switches the terminal to this project too); it exists
			// now, so the original command works.
			out, err = c.R.Run(ctx, NewWindowArgs(c.session(), dir))
		}
		if err != nil {
			return 0, err
		}
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, errors.New("tmux did not print a window index")
	}
	return idx, nil
}

// sessionMissing reports whether err means "this client's session does not
// exist yet," for the one control command that cannot say so the normal way.
// new-window's target has no ":index" suffix, and tmux's target resolver
// reports an unmatched bare name as a missing *window*, not a missing
// session -- "can't find window: homebase-xxx" rather than "can't find
// session: homebase-xxx". The comparison is exact and keyed to this client's
// own session name specifically so it can never be tripped by an unrelated
// "can't find window: <index>" error against a real window index, e.g. from
// KillWindow or SelectWindow targeting a stale index in a session that does
// exist.
func (c Client) sessionMissing(err error) bool {
	if errors.Is(err, ErrNoSession) {
		return true
	}
	return err.Error() == fmt.Sprintf("tmux: can't find window: %s", c.session())
}

// startDir resolves the directory a new window should open in. A missing
// session, or any other lookup failure, falls back to $HOME rather than
// failing window creation over it.
func (c Client) startDir(ctx context.Context, dirMode string) string {
	home, _ := os.UserHomeDir()
	if dirMode == "home" {
		return home
	}
	out, err := c.R.Run(ctx, CurrentPathArgs(c.session()))
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
	_, err := c.R.Run(ctx, RenameWindowArgs(c.session(), index, name))
	return err
}

// KillWindow removes a window. In the legacy singleton session it refuses
// the last one, because tmux would destroy the session along with it — see
// AGENT.md hard constraint 3. A per-project session has no such guard: its
// existence is tracked in projects.json, not by keeping a window alive, so
// closing the last window is allowed and simply ends that tmux session; the
// next window reopens it.
func (c Client) KillWindow(ctx context.Context, index int) error {
	if c.session() == SessionName {
		windows, err := c.ListWindows(ctx)
		if err != nil {
			return err
		}
		if len(windows) <= 1 {
			return ErrLastWindow
		}
	}
	_, err := c.R.Run(ctx, KillWindowArgs(c.session(), index))
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
		_, _ = c.R.Run(ctx, CancelCopyModeArgs(c.session()))
		return false, nil
	}
	if _, err := c.R.Run(ctx, CopyModeArgs(c.session())); err != nil {
		return false, err
	}
	if _, err := c.R.Run(ctx, ScrollArgs(c.session(), lines)); err != nil {
		return false, err
	}
	out, err := c.R.Run(ctx, InModeArgs(c.session()))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "1", nil
}

// SelectWindow makes a window current for the session, which is what the
// attached PTY redraws.
func (c Client) SelectWindow(ctx context.Context, index int) error {
	_, err := c.R.Run(ctx, SelectWindowArgs(c.session(), index))
	return err
}
