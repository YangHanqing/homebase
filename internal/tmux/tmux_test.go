package tmux

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestAttachArgsKeepsSemicolonSeparate(t *testing.T) {
	args := AttachArgs("")
	found := -1
	for i, a := range args {
		if a == ";" {
			found = i
		}
	}
	if found < 0 {
		t.Fatalf("no standalone %q in %q", ";", args)
	}
	// Folding the ";" into a neighbour would make tmux read it as part of the
	// session name.
	if args[found-1] != SessionName {
		t.Fatalf("';' should follow the session name, got %q", args)
	}
	if strings.Join(args, " ") != "new-session -A -s homebase ; set-option -t homebase status off" {
		t.Fatalf("unexpected attach argv: %q", args)
	}
}

func TestAttachArgsWithDirInsertsDashC(t *testing.T) {
	args := AttachArgs("/tmp/x")
	if strings.Join(args, " ") != "new-session -A -s homebase -c /tmp/x ; set-option -t homebase status off" {
		t.Fatalf("unexpected attach argv: %q", args)
	}
}

func TestNewWindowArgsWithDirInsertsDashC(t *testing.T) {
	if got := strings.Join(NewWindowArgs(""), " "); got != "new-window -t homebase -P -F #{window_index}" {
		t.Fatalf("unexpected new-window argv: %q", got)
	}
	if got := strings.Join(NewWindowArgs("/tmp/x"), " "); got != "new-window -t homebase -c /tmp/x -P -F #{window_index}" {
		t.Fatalf("unexpected new-window argv: %q", got)
	}
}

func TestParseWindows(t *testing.T) {
	out := []byte("0 0 zsh\n1 1 my work\n2 0 vim\n")
	got := parseWindows(out)
	want := []Window{
		{Index: 0, Name: "zsh", Active: false},
		{Index: 1, Name: "my work", Active: true},
		{Index: 2, Name: "vim", Active: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if w := parseWindows(nil); len(w) != 0 || w == nil {
		t.Fatalf("empty output should give an empty non-nil slice, got %#v", w)
	}
}

func TestClassifyOutput(t *testing.T) {
	if err := classifyOutput([]byte(ENOTMUXMarker+"\n"), nil, errors.New("exit 127")); !errors.Is(err, ErrNoTmux) {
		t.Fatalf("want ErrNoTmux, got %v", err)
	}
	if err := classifyOutput(nil, []byte("can't find session: homebase\n"), errors.New("exit 1")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
	if err := classifyOutput(nil, []byte("no server running on /tmp/x\n"), errors.New("exit 1")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
	if err := classifyOutput([]byte("0 1 zsh\n"), nil, nil); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

// fakeRunner records argv and replays canned output.
type fakeRunner struct {
	out  map[string]string
	err  map[string]error
	seen [][]string
}

func (f *fakeRunner) Run(_ context.Context, args []string) ([]byte, error) {
	f.seen = append(f.seen, args)
	key := args[0]
	if err, ok := f.err[key]; ok {
		return nil, err
	}
	return []byte(f.out[key]), nil
}

func TestListWindowsTreatsMissingSessionAsEmpty(t *testing.T) {
	c := Client{R: &fakeRunner{err: map[string]error{"list-windows": ErrNoSession}}}
	got, err := c.ListWindows(context.Background())
	if err != nil {
		t.Fatalf("a session that does not exist yet is not an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestKillWindowRefusesTheLastOne(t *testing.T) {
	f := &fakeRunner{out: map[string]string{"list-windows": "0 1 zsh\n"}}
	c := Client{R: f}
	if err := c.KillWindow(context.Background(), 0); !errors.Is(err, ErrLastWindow) {
		t.Fatalf("want ErrLastWindow, got %v", err)
	}
	for _, args := range f.seen {
		if args[0] == "kill-window" {
			t.Fatal("kill-window must not have been issued")
		}
	}

	f2 := &fakeRunner{out: map[string]string{"list-windows": "0 1 zsh\n1 0 vim\n"}}
	if err := (Client{R: f2}).KillWindow(context.Background(), 1); err != nil {
		t.Fatalf("killing a non-last window: %v", err)
	}
	if f2.seen[len(f2.seen)-1][0] != "kill-window" {
		t.Fatalf("expected kill-window, got %q", f2.seen)
	}
}

func TestNewWindowParsesIndex(t *testing.T) {
	c := Client{R: &fakeRunner{out: map[string]string{"new-window": "3\n"}}}
	idx, err := c.NewWindow(context.Background())
	if err != nil || idx != 3 {
		t.Fatalf("got %d %v", idx, err)
	}
}

func TestNewWindowStartsInHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	f := &fakeRunner{out: map[string]string{"new-window": "1\n"}}
	c := Client{R: f}
	if _, err := c.NewWindow(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, args := range f.seen {
		if args[0] == "new-window" {
			got = args
		}
	}
	if !contains(got, "-c") || !contains(got, home) {
		t.Fatalf("new-window should start in home %q, got %q", home, got)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
