package tmux

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestAttachArgsKeepsSemicolonSeparate(t *testing.T) {
	args := AttachArgs(SessionName, "")
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
	args := AttachArgs(SessionName, "/tmp/x")
	if strings.Join(args, " ") != "new-session -A -s homebase -c /tmp/x ; set-option -t homebase status off" {
		t.Fatalf("unexpected attach argv: %q", args)
	}
}

func TestNewWindowArgsWithDirInsertsDashC(t *testing.T) {
	if got := strings.Join(NewWindowArgs(SessionName, ""), " "); got != "new-window -t homebase -P -F #{window_index}" {
		t.Fatalf("unexpected new-window argv: %q", got)
	}
	if got := strings.Join(NewWindowArgs(SessionName, "/tmp/x"), " "); got != "new-window -t homebase -c /tmp/x -P -F #{window_index}" {
		t.Fatalf("unexpected new-window argv: %q", got)
	}
}

// The order of the fields in listFormat is a contract with parseWindows:
// every machine-readable field first, the free-form name last, single spaces
// between. A name may contain spaces, so anything appended after it would be
// swallowed by the name.
func TestListFormatKeepsTheNameLast(t *testing.T) {
	want := "#{window_index} #{window_active} #{window_activity} #{window_bell_flag} #{window_name}"
	if listFormat != want {
		t.Fatalf("listFormat = %q, want %q", listFormat, want)
	}
	if strings.ContainsAny(listFormat, "\t\r\n") {
		t.Fatal("listFormat must not use a control character as a separator")
	}
	if !strings.HasSuffix(listFormat, "#{window_name}") {
		t.Fatal("the free-form window name must be the last field")
	}
	if got := strings.Join(ListArgs(SessionName), " "); got != "list-windows -t homebase -F "+listFormat {
		t.Fatalf("unexpected list-windows argv: %q", got)
	}
}

func TestParseWindows(t *testing.T) {
	out := []byte("0 0 1787485588 0 zsh\n1 1 1787485591 0 my work\n2 0 1787480000 1 vim\n")
	got := parseWindows(out)
	want := []Window{
		{Index: 0, Name: "zsh", Active: false, Activity: 1787485588},
		{Index: 1, Name: "my work", Active: true, Activity: 1787485591},
		{Index: 2, Name: "vim", Active: false, Activity: 1787480000, Bell: true},
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

// A short or garbled line must not take a window off the list any more than
// it already did: a truncated row is dropped, but a row whose only problem is
// an unreadable timestamp keeps the window and loses only the signal.
func TestParseWindowsIsDefensive(t *testing.T) {
	out := []byte("0 0 1787485588 0 zsh\n1 1 vim\n\nbad 0 1 0 x\n3 0 nope 0 a name with spaces\n")
	got := parseWindows(out)
	want := []Window{
		{Index: 0, Name: "zsh", Activity: 1787485588},
		{Index: 3, Name: "a name with spaces", Activity: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestClassifyOutput(t *testing.T) {
	if err := classifyOutput(nil, []byte("can't find session: homebase\n"), errors.New("exit 1")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
	if err := classifyOutput(nil, []byte("no server running on /tmp/x\n"), errors.New("exit 1")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
	if err := classifyOutput(nil, []byte("error connecting to /tmp/x (No such file or directory)\n"), errors.New("exit 1")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
	if err := classifyOutput([]byte("0 1 zsh\n"), nil, nil); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	// stdout is data, not diagnostics: window names are user-chosen and come
	// back verbatim from list-windows -F. A window named after one of tmux's
	// own complaints must not be mistaken for that complaint.
	if err := classifyOutput([]byte("0 1 no server running\n"), nil, nil); err != nil {
		t.Fatalf("a window name must not classify as an error, got %v", err)
	}
	if err := classifyOutput([]byte("0 1 can't find session\n"), nil, nil); err != nil {
		t.Fatalf("a window name must not classify as an error, got %v", err)
	}
}

// A successful list whose payload happens to quote tmux's own error text must
// survive intact, all the way through the Client.
func TestListWindowsKeepsWindowsNamedAfterTmuxErrors(t *testing.T) {
	c := Client{R: classifyingRunner{out: "0 1 1787485588 0 no server running\n1 0 1787485500 0 zsh\n"}}
	got, err := c.ListWindows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want both windows", got)
	}
	if got[0].Name != "no server running" {
		t.Errorf("name mangled: %+v", got[0])
	}
}

// classifyingRunner mirrors LocalRunner's exact pipeline: hand the command's
// streams and exit status to classifyOutput, then return whatever it says.
type classifyingRunner struct {
	out    string
	errOut string
	exit   error
}

func (r classifyingRunner) Run(_ context.Context, _ []string) ([]byte, error) {
	if err := classifyOutput([]byte(r.out), []byte(r.errOut), r.exit); err != nil {
		return nil, err
	}
	return []byte(r.out), nil
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
	f := &fakeRunner{out: map[string]string{"list-windows": "0 1 1787485588 0 zsh\n"}}
	c := Client{R: f}
	if err := c.KillWindow(context.Background(), 0); !errors.Is(err, ErrLastWindow) {
		t.Fatalf("want ErrLastWindow, got %v", err)
	}
	for _, args := range f.seen {
		if args[0] == "kill-window" {
			t.Fatal("kill-window must not have been issued")
		}
	}

	f2 := &fakeRunner{out: map[string]string{"list-windows": "0 1 1787485588 0 zsh\n1 0 1787485500 0 vim\n"}}
	if err := (Client{R: f2}).KillWindow(context.Background(), 1); err != nil {
		t.Fatalf("killing a non-last window: %v", err)
	}
	if f2.seen[len(f2.seen)-1][0] != "kill-window" {
		t.Fatalf("expected kill-window, got %q", f2.seen)
	}
}

func TestNewWindowParsesIndex(t *testing.T) {
	c := Client{R: &fakeRunner{out: map[string]string{"new-window": "3\n"}}}
	idx, err := c.NewWindow(context.Background(), "home")
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
	if _, err := c.NewWindow(context.Background(), "home"); err != nil {
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

func TestNewWindowDefaultsToCurrentPaneDir(t *testing.T) {
	f := &fakeRunner{out: map[string]string{
		"new-window":      "1\n",
		"display-message": "/tmp/proj\n",
	}}
	c := Client{R: f}
	if _, err := c.NewWindow(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, args := range f.seen {
		if args[0] == "new-window" {
			got = args
		}
	}
	if !contains(got, "-c") || !contains(got, "/tmp/proj") {
		t.Fatalf("new-window should start in the current pane's directory, got %q", got)
	}
}

func TestNewWindowFallsBackToHomeWhenCurrentPathFails(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	f := &fakeRunner{
		out: map[string]string{"new-window": "1\n"},
		err: map[string]error{"display-message": ErrNoSession},
	}
	c := Client{R: f}
	if _, err := c.NewWindow(context.Background(), "same"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, args := range f.seen {
		if args[0] == "new-window" {
			got = args
		}
	}
	if !contains(got, "-c") || !contains(got, home) {
		t.Fatalf("new-window should fall back to home %q, got %q", home, got)
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
