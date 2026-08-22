package tmux

import (
	"context"
	"strings"
	"testing"
)

func TestScrollArgs(t *testing.T) {
	for _, tc := range []struct {
		lines int
		want  string
	}{
		{5, "send-keys -t homebase -X -N 5 scroll-up"},
		{-5, "send-keys -t homebase -X -N 5 scroll-down"},
		{1, "send-keys -t homebase -X -N 1 scroll-up"},
		// tmux clamps at the ends of its history; the cap is only there so a
		// bad client cannot make us build an absurd repeat count.
		{100000, "send-keys -t homebase -X -N 500 scroll-up"},
		{-100000, "send-keys -t homebase -X -N 500 scroll-down"},
	} {
		if got := strings.Join(ScrollArgs(tc.lines), " "); got != tc.want {
			t.Errorf("ScrollArgs(%d) = %q, want %q", tc.lines, got, tc.want)
		}
	}
}

// "-e" is what makes swiping back down return to the live shell with nothing
// to dismiss. Without it the pane stays in copy mode forever.
func TestCopyModeExitsAtTheBottom(t *testing.T) {
	if got := strings.Join(CopyModeArgs(), " "); got != "copy-mode -e -t homebase" {
		t.Errorf("CopyModeArgs = %q", got)
	}
}

type scriptRunner struct {
	calls [][]string
	out   map[string]string
}

func (r *scriptRunner) Run(_ context.Context, args []string) ([]byte, error) {
	r.calls = append(r.calls, args)
	return []byte(r.out[args[0]]), nil
}

// send-keys -X outside copy mode fails with "not in a mode", so every scroll
// has to enter copy mode first. Re-entering keeps the scroll position, which
// is what lets this stay stateless.
func TestScrollEntersCopyModeFirst(t *testing.T) {
	r := &scriptRunner{out: map[string]string{"display-message": "1\n"}}
	inMode, err := Client{R: r}.Scroll(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !inMode {
		t.Error("pane reported out of copy mode despite scrolling back")
	}
	if len(r.calls) != 3 || r.calls[0][0] != "copy-mode" || r.calls[1][0] != "send-keys" {
		t.Fatalf("wrong command order: %v", r.calls)
	}
}

func TestScrollZeroCancels(t *testing.T) {
	r := &scriptRunner{out: map[string]string{}}
	inMode, err := Client{R: r}.Scroll(context.Background(), 0)
	if err != nil || inMode {
		t.Fatalf("inMode=%v err=%v", inMode, err)
	}
	if len(r.calls) != 1 || strings.Join(r.calls[0], " ") != "send-keys -t homebase -X cancel" {
		t.Fatalf("want a single cancel, got %v", r.calls)
	}
}
