package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// privateSocketDir points tmux at a throwaway server and returns its
// TMUX_TMPDIR. The directory is short because t.TempDir() embeds the test name
// and easily blows past the ~104-byte sun_path limit for tmux's socket.
//
// Clearing TMUX is the load-bearing half. tmux prefers the socket named in
// $TMUX over TMUX_TMPDIR, and the obvious way to work on this project is from
// inside the homebase session itself — so without this every live test either
// skipped with "duplicate session: homebase" or, on a machine with no session
// yet, ran the whole control-channel suite (rename, kill, scroll) against the
// operator's real tmux server and then cleaned up a different socket.
func privateSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("TMUX_TMPDIR", dir)
	// t.Setenv registers the restore; Unsetenv makes it actually absent,
	// which is what tmux checks.
	for _, k := range []string{"TMUX", "TMUX_PANE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	return dir
}

// killPrivateServer tears down the throwaway server at the end of a test.
// It names the socket explicitly rather than trusting TMUX_TMPDIR: tmux
// silently falls back to the default socket when TMUX_TMPDIR points at a
// directory that is not there, and a kill-server that lands on the default
// socket destroys the operator's own homebase session. With -S a stale path
// is simply an error.
func killPrivateServer(t *testing.T, bin, dir string) {
	t.Helper()
	socket := filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()), "default")
	t.Cleanup(func() { _ = exec.Command(bin, "-S", socket, "kill-server").Run() })
}

// liveClient runs against a real tmux, but on a private server socket
// (TMUX_TMPDIR) so it can never touch the operator's own homebase session.
func liveClient(t *testing.T) Client {
	t.Helper()
	bin, err := LocalBinary()
	if err != nil {
		t.Skip("tmux not installed")
	}
	dir := privateSocketDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "new-session", "-d", "-s", SessionName).CombinedOutput(); err != nil {
		t.Skipf("cannot start a private tmux server: %v (%s)", err, out)
	}
	killPrivateServer(t, bin, dir)
	return Client{R: LocalRunner{Bin: bin}}
}

func TestLiveControlChannel(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	got, err := c.ListWindows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("fresh session should have one window, got %+v", got)
	}

	idx, err := c.NewWindow(ctx, "home", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RenameWindow(ctx, idx, "my work"); err != nil {
		t.Fatal(err)
	}
	got, err = c.ListWindows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 windows, got %+v", got)
	}
	if find(got, idx).Name != "my work" {
		t.Fatalf("rename did not stick: %+v", got)
	}

	if err := c.SelectWindow(ctx, idx); err != nil {
		t.Fatal(err)
	}
	if got, _ = c.ListWindows(ctx); !find(got, idx).Active {
		t.Fatalf("select-window did not take: %+v", got)
	}

	if err := c.KillWindow(ctx, idx); err != nil {
		t.Fatal(err)
	}
	got, _ = c.ListWindows(ctx)
	if len(got) != 1 {
		t.Fatalf("want 1 window after kill, got %+v", got)
	}

	// The last one would take the session down with it.
	if err := c.KillWindow(ctx, got[0].Index); !errors.Is(err, ErrLastWindow) {
		t.Fatalf("want ErrLastWindow, got %v", err)
	}
	if got, _ = c.ListWindows(ctx); len(got) != 1 {
		t.Fatalf("session should have survived, got %+v", got)
	}
}

// TestLiveNewWindowStartsInHome is the regression for launchd: the server
// process cwd is often "/", and a new window without -c would land there.
func TestLiveNewWindowStartsInHome(t *testing.T) {
	bin, err := LocalBinary()
	if err != nil {
		t.Skip("tmux not installed")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	want, err := filepath.EvalSymlinks(home)
	if err != nil {
		want = home
	}

	dir := privateSocketDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "new-session", "-d", "-s", SessionName, "-c", "/tmp").CombinedOutput(); err != nil {
		t.Skipf("cannot start a private tmux server: %v (%s)", err, out)
	}
	killPrivateServer(t, bin, dir)

	c := Client{R: LocalRunner{Bin: bin}}
	idx, err := c.NewWindow(ctx, "home", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(ctx, bin,
		"display-message", "-p", "-t", target(SessionName, idx), "#{pane_current_path}").Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(out))
	gotResolved, err := filepath.EvalSymlinks(got)
	if err == nil {
		got = gotResolved
	}
	if got != want {
		t.Fatalf("new window cwd: got %q, want home %q", got, want)
	}
}

func find(ws []Window, index int) Window {
	for _, w := range ws {
		if w.Index == index {
			return w
		}
	}
	return Window{}
}

// TestLiveAttachTurnsStatusBarOff proves the UI promise: the user never sees
// tmux's own bottom bar, and turning it off for our session does not touch the
// global default that the operator's other tmux sessions inherit.
func TestLiveAttachTurnsStatusBarOff(t *testing.T) {
	bin, err := LocalBinary()
	if err != nil {
		t.Skip("tmux not installed")
	}
	dir := privateSocketDir(t)
	killPrivateServer(t, bin, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Same argv the PTY channel uses, minus the attach (-d keeps it headless).
	args := append([]string{"new-session", "-d"}, AttachArgs(SessionName, "")[1:]...)
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot start a private tmux server: %v (%s)", err, out)
	}

	show := func(scope ...string) string {
		out, err := exec.CommandContext(ctx, bin, append([]string{"show-options"}, scope...)...).Output()
		if err != nil {
			t.Fatalf("show-options %v: %v", scope, err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := show("-t", SessionName, "status"); got != "status off" {
		t.Fatalf("session status: %q", got)
	}
	if got := show("-g", "status"); got != "status on" {
		t.Fatalf("global status was clobbered: %q", got)
	}
}

// TestLiveListSurvivesEmptyLocale is the regression test for the launchd bug:
// with no LANG in the environment tmux rewrites control characters in -F
// output (a tab became "_"), which silently emptied the window list. ExecEnv
// now supplies a UTF-8 locale and listFormat no longer relies on a tab.
func TestLiveListSurvivesEmptyLocale(t *testing.T) {
	bin, err := LocalBinary()
	if err != nil {
		t.Skip("tmux not installed")
	}
	dir := privateSocketDir(t)
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	killPrivateServer(t, bin, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "new-session", "-d", "-s", SessionName).CombinedOutput(); err != nil {
		t.Skipf("cannot start a private tmux server: %v (%s)", err, out)
	}

	c := Client{R: LocalRunner{Bin: bin}}
	if err := c.RenameWindow(ctx, 0, "my work"); err != nil {
		t.Fatal(err)
	}
	windows, err := c.ListWindows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 {
		t.Fatalf("got %+v", windows)
	}
	if windows[0].Name != "my work" || !windows[0].Active {
		t.Fatalf("window %+v", windows[0])
	}
}

// The whole scroll feature rests on tmux behaviour that is easy to get wrong:
// send-keys -X fails outside copy mode, re-entering copy mode must not reset
// the position, and "copy-mode -e" must drop out of copy mode by itself once
// the view is back at the bottom.
func TestLiveScrollWalksTheScrollback(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	fillHistory(t, c)

	pos := func() string {
		out, err := c.R.Run(ctx, []string{"display-message", "-p", "-t", SessionName, "#{scroll_position}"})
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}

	inMode, err := c.Scroll(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !inMode {
		t.Fatal("scrolling back should leave the pane in copy mode")
	}
	if got := pos(); got != "10" {
		t.Fatalf("scroll_position = %q, want 10", got)
	}

	// Re-entering copy mode for the next swipe must not snap back to the
	// bottom, or a slow drag would fight itself.
	if _, err := c.Scroll(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if got := pos(); got != "15" {
		t.Fatalf("scroll_position = %q after a second scroll, want 15", got)
	}

	// Swiping back down past the bottom is the only exit the UI offers.
	inMode, err = c.Scroll(ctx, -100)
	if err != nil {
		t.Fatal(err)
	}
	if inMode {
		t.Error("copy-mode -e must exit on its own at the bottom")
	}
}

// A tap has to get back to a live prompt even when the user never swiped all
// the way down, and cancelling when there is nothing to cancel is not an error.
func TestLiveScrollZeroCancelsFromAnywhere(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	if inMode, err := c.Scroll(ctx, 0); err != nil || inMode {
		t.Fatalf("cancel outside copy mode: inMode=%v err=%v", inMode, err)
	}
	fillHistory(t, c)
	if _, err := c.Scroll(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if inMode, err := c.Scroll(ctx, 0); err != nil || inMode {
		t.Fatalf("cancel from copy mode: inMode=%v err=%v", inMode, err)
	}
}

// fillHistory prints enough lines to scroll through, then waits for them to
// land. A fixed sleep is not enough: the pane's shell has to finish starting
// before it will run anything, and until it does history_size stays 0 and
// every scroll silently does nothing.
func fillHistory(t *testing.T, c Client) {
	t.Helper()
	ctx := context.Background()
	if _, err := c.R.Run(ctx, []string{"send-keys", "-t", SessionName, "seq 1 300", "Enter"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, err := c.R.Run(ctx, []string{"display-message", "-p", "-t", SessionName, "#{history_size}"})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && n >= 100 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Skip("pane never produced scrollback; shell too slow to start")
}

// The list must survive a window named after one of tmux's own complaints.
// Window names are user-chosen and `list-windows -F` prints them straight
// back, so a classifier that reads stdout turns a healthy session into
// "no session": every window disappears from the sidebar, and KillWindow then
// refuses each delete as "last window" because the list it checks is empty.
func TestLiveWindowNamedAfterATmuxErrorDoesNotEmptyTheList(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	idx, err := c.NewWindow(ctx, "home", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"no server running", "can't find session", "no such session"} {
		if err := c.RenameWindow(ctx, idx, name); err != nil {
			t.Fatalf("rename to %q: %v", name, err)
		}
		got, err := c.ListWindows(ctx)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if len(got) != 2 {
			t.Fatalf("%q ate the window list: %+v", name, got)
		}
		if find(got, idx).Name != name {
			t.Fatalf("%q: name mangled: %+v", name, got)
		}
		// The count feeds the same "someone else is attached" warning, and
		// took the same path through the classifier.
		if n, err := c.ClientCount(ctx); err != nil {
			t.Fatalf("%q: client count: %v", name, err)
		} else if n != 0 {
			t.Fatalf("%q: detached server should report 0 clients, got %d", name, n)
		}
		// Deleting must stay possible: with an empty list this returned
		// ErrLastWindow instead.
		if err := c.KillWindow(ctx, idx); err != nil {
			t.Fatalf("%q: kill refused: %v", name, err)
		}
		if idx, err = c.NewWindow(ctx, "home", ""); err != nil {
			t.Fatal(err)
		}
	}
}

// The sidebar's activity dot rests on one tmux fact that is worth pinning
// against a real server: #{window_activity} advances for a window nobody is
// looking at, with monitor-activity left alone.
//
// The tempting alternative — #{window_activity_flag} plus "set-option -t
// homebase monitor-activity on" — does not work. monitor-activity is a
// *window* option, so a session target sets it on that session's current
// window only and later windows never inherit it; the session-wide form
// would leak into the user's other sessions on the same tmux server. This
// test asserts the option is still off precisely so a future "fix" that
// turns it on has to explain itself.
func TestLiveBackgroundWindowActivityAdvances(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	// Window 0 stops being the current one as soon as this lands, so what
	// follows is measured on a window with no viewer at all.
	if _, err := c.NewWindow(ctx, "home", ""); err != nil {
		t.Fatal(err)
	}
	if out, err := c.R.Run(ctx, []string{"show-options", "-w", "-t", SessionName + ":0", "monitor-activity"}); err != nil {
		t.Fatal(err)
	} else if got := strings.TrimSpace(string(out)); got == "monitor-activity on" {
		t.Fatal("monitor-activity must stay untouched; the activity timestamp does not need it")
	}

	before := find(mustList(t, c, ctx), 0)
	if before.Activity == 0 {
		t.Fatal("a live window should carry an activity timestamp")
	}
	if before.Active {
		t.Fatal("window 0 should be in the background for this test")
	}

	// Unix-second granularity: without the wait the new output can land in
	// the same second and the timestamp legitimately does not move.
	time.Sleep(1100 * time.Millisecond)
	if _, err := c.R.Run(ctx, []string{"send-keys", "-t", SessionName + ":0", "echo homebase-activity", "Enter"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		after := find(mustList(t, c, ctx), 0)
		if after.Activity > before.Activity {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("background window activity never advanced: %d -> %d", before.Activity, after.Activity)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func mustList(t *testing.T, c Client, ctx context.Context) []Window {
	t.Helper()
	got, err := c.ListWindows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
