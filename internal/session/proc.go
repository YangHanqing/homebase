package session

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const killGrace = 2 * time.Second

type proc struct {
	cmd    *exec.Cmd
	file   *os.File
	stderr *limitedBuf
	mu     sync.Mutex
	killed bool

	// reaped is closed once Wait has returned, which is the moment the pid
	// (and its pgid) become reusable by the kernel. Kill's delayed SIGKILL
	// waits on it so the escalation cannot land on an unrelated process
	// group that happened to inherit the number.
	reaped     chan struct{}
	reapedOnce sync.Once
}

func newProc(cmd *exec.Cmd, file *os.File, stderr *limitedBuf) *proc {
	return &proc{cmd: cmd, file: file, stderr: stderr, reaped: make(chan struct{})}
}

func (p *proc) Read(b []byte) (int, error) {
	if p.file == nil {
		return 0, io.EOF
	}
	return p.file.Read(b)
}

func (p *proc) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.killed || p.file == nil {
		return 0, io.ErrClosedPipe
	}
	return p.file.Write(b)
}

func (p *proc) SetSize(sz Size) error {
	if !sz.Valid() {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.killed || p.file == nil {
		return io.ErrClosedPipe
	}
	return pty.Setsize(p.file, &pty.Winsize{Rows: sz.Rows, Cols: sz.Cols})
}

func (p *proc) Wait() error {
	if p.cmd == nil {
		return nil
	}
	err := p.cmd.Wait()
	p.reapedOnce.Do(func() { close(p.reaped) })
	return err
}

func (p *proc) Kill() error {
	p.mu.Lock()
	if p.killed {
		p.mu.Unlock()
		return nil
	}
	p.killed = true
	file := p.file
	cmd := p.cmd
	p.mu.Unlock()

	if file != nil {
		_ = file.Close()
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	go func() {
		timer := time.NewTimer(killGrace)
		defer timer.Stop()
		select {
		case <-p.reaped:
			// Already exited and reaped; the pgid may belong to someone else now.
		case <-timer.C:
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}()
	return nil
}

func (p *proc) Stderr() []byte {
	if p.stderr == nil {
		return nil
	}
	return p.stderr.Bytes()
}

// limitedBuf keeps a cap on captured ssh stderr so we never retain a secret dump.
type limitedBuf struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newLimitedBuf(n int) *limitedBuf {
	return &limitedBuf{cap: n}
}

func (b *limitedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.buf) >= b.cap {
		return len(p), nil
	}
	need := b.cap - len(b.buf)
	if len(p) < need {
		need = len(p)
	}
	b.buf = append(b.buf, p[:need]...)
	return len(p), nil
}

func (b *limitedBuf) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.buf))
	copy(out, b.buf)
	return out
}
