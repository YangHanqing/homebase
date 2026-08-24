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
	home, _ := os.UserHomeDir()
	cmd := exec.Command(bin, tmux.AttachArgs(d.session(), home)...)
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
