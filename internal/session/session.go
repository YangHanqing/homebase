// Package session owns the local PTY + tmux process.
package session

import "context"

// Size is a PTY winsize. Valid range is 1–512 on each axis.
type Size struct {
	Cols uint16
	Rows uint16
}

// Valid reports whether cols/rows are in the protocol range.
func (s Size) Valid() bool {
	return s.Cols >= 1 && s.Cols <= 512 && s.Rows >= 1 && s.Rows <= 512
}

func (s Size) orDefault() Size {
	if s.Valid() {
		return s
	}
	return Size{Cols: 80, Rows: 24}
}

// Proc is one local process attached to a PTY.
type Proc interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	SetSize(Size) error
	Wait() error
	Kill() error
}

// Stderrer is optionally implemented by Proc to expose ssh stderr (never PTY bytes).
type Stderrer interface {
	Stderr() []byte
}

// Dialer starts the PTY channel.
type Dialer interface {
	Start(ctx context.Context, sz Size) (Proc, error)
}
