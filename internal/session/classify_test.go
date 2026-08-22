package session

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yanghanqing/homebase/internal/tmux"
)

// A missing tmux is the one spawn failure with an action the user can take, so
// it must not be flattened into the generic pty_spawn the UI has no hint for.
func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stderr   []byte
		spawnErr error
		wantCode string
	}{
		{"missing tmux", nil, fmt.Errorf("local: %w", tmux.ErrNoTmux), CodeENOTMUX},
		{"other spawn failure", nil, errors.New("fork/exec: permission denied"), CodePTYSpawn},
		{"clean exit", nil, nil, ""},
		{"blank stderr is not a failure", []byte("  \n\t "), nil, ""},
		{"unclassified stderr", []byte("something broke\nline two\n"), nil, CodeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, msg := Classify(tc.stderr, tc.spawnErr)
			if code != tc.wantCode {
				t.Fatalf("code = %q, want %q (msg %q)", code, tc.wantCode, msg)
			}
			if strings.ContainsAny(msg, "\r\n") {
				t.Fatalf("message must be one line, got %q", msg)
			}
		})
	}
}

// The message is user-visible; a runaway stderr must not become the status bar.
func TestClassifyTrimsLongMessages(t *testing.T) {
	_, msg := Classify([]byte(strings.Repeat("x", 5000)), nil)
	if len(msg) > maxMessage {
		t.Fatalf("message is %d bytes, want <= %d", len(msg), maxMessage)
	}
}
