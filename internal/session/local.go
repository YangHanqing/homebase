package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/yanghanqing/homebase/internal/tmux"
)

// LocalDialer attaches to tmux on the machine Homebase itself runs on.
// No ssh, no keys, no known_hosts — it is simply a child process.
type LocalDialer struct {
	// Tmux overrides tmux discovery in tests only.
	Tmux string
	// Session is the tmux session to attach. Empty means the legacy
	// singleton session (tmux.SessionName), so a zero-value LocalDialer keeps
	// working exactly as it did before per-project sessions existed.
	Session string
	// StartDir is the start directory for the session when this attach is the
	// call that creates it (post-reboot, or after the session was killed).
	// "new-session -A" ignores -c for an existing session, so this only ever
	// bites the first attach. Empty falls back to $HOME, matching the legacy
	// singleton, which has no project path; a project dialer sets it to the
	// project's own path so that session's first window lands there.
	StartDir string
}

func (d LocalDialer) session() string {
	if d.Session == "" {
		return tmux.SessionName
	}
	return d.Session
}

// Start implements Dialer.
func (d LocalDialer) Start(ctx context.Context, sz Size) (Proc, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bin := d.Tmux
	if bin == "" {
		found, err := tmux.LocalBinary()
		if err != nil {
			return nil, err
		}
		bin = found
	}
	sz = sz.orDefault()
	dir := d.StartDir
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	cmd := exec.Command(bin, tmux.AttachArgs(d.session(), dir)...)
	// creack/pty sets Setsid+Setctty. Setpgid together with that is EPERM on macOS.
	cmd.Env = tmux.ExecEnv()
	stderr := newLimitedBuf(4096)
	cmd.Stderr = stderr

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: sz.Rows, Cols: sz.Cols})
	if err != nil {
		return nil, fmt.Errorf("could not start tmux: %w", err)
	}
	p := newProc(cmd, ptmx, stderr)
	go func() {
		<-ctx.Done()
		_ = p.Kill()
	}()
	return p, nil
}
