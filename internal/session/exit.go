package session

import (
	"errors"
	"os/exec"
	"syscall"
)

// ExitDetail describes how a PTY process ended. It exists for the log line
// only: a session that vanished used to leave nothing behind but "ws close",
// which is exactly what made the disappearing-tmux reports undiagnosable.
type ExitDetail struct {
	// Code is the process exit status, or -1 when it never exited normally
	// (killed by a signal, or Wait failed for another reason).
	Code int
	// Signal is set only when the kernel killed the process. Homebase sends
	// SIGTERM to the group itself on close, so "terminated" here is usually
	// our own doing; anything else came from outside.
	Signal string
	// Err carries Wait's error when it was not an *exec.ExitError.
	Err string
}

// DescribeExit turns a Proc.Wait error into loggable detail.
func DescribeExit(err error) ExitDetail {
	if err == nil {
		return ExitDetail{}
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return ExitDetail{Code: -1, Err: trimMessage(err.Error())}
	}
	d := ExitDetail{Code: ee.ExitCode()}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		d.Signal = ws.Signal().String()
	}
	return d
}

// LogArgs renders the detail as slog key/value pairs, omitting the fields
// that carry no information.
func (d ExitDetail) LogArgs() []any {
	args := []any{"exit_code", d.Code}
	if d.Signal != "" {
		args = append(args, "signal", d.Signal)
	}
	if d.Err != "" {
		args = append(args, "wait_err", d.Err)
	}
	return args
}

// StderrSnippet caps captured stderr for a log line. Same bound as the
// message on the status frame, for the same reason: never dump a stream.
func StderrSnippet(b []byte) string {
	return trimMessage(string(b))
}
