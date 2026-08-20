package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// privateSocketDir returns a short TMUX_TMPDIR. t.TempDir() embeds the test
// name and easily blows past the ~104-byte sun_path limit for tmux's socket.
func privateSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// liveClient runs against a real tmux, but on a private server socket
// (TMUX_TMPDIR) so it can never touch the operator's own homebase session.
func liveClient(t *testing.T) Client {
	t.Helper()
	bin, err := LocalBinary()
	if err != nil {
		t.Skip("tmux not installed")
	}
	t.Setenv("TMUX_TMPDIR", privateSocketDir(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "new-session", "-d", "-s", SessionName).CombinedOutput(); err != nil {
		t.Skipf("cannot start a private tmux server: %v (%s)", err, out)
	}
	t.Cleanup(func() {
		// Our own throwaway server, not the operator's.
		_ = exec.Command(bin, "kill-server").Run()
	})
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

	idx, err := c.NewWindow(ctx)
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

// TestLiveNewWindowInheritsActivePanePath is the regression test for the real
// bug: without -c, tmux gives a new window the cwd of whatever process ran
// `new-window` — the homebase server — not the directory the user is actually
// sitting in. The test's own cwd is the package directory, so a new window
// landing in dir proves the path came from the active pane and not from us.
func TestLiveNewWindowInheritsActivePanePath(t *testing.T) {
	bin, err := LocalBinary()
	if err != nil {
		t.Skip("tmux not installed")
	}
	dir, err := os.MkdirTemp("", "hbcwd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	// macOS hands out /var/... symlinked to /private/var; tmux reports the
	// resolved form, so compare against that.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("TMUX_TMPDIR", privateSocketDir(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "new-session", "-d", "-s", SessionName, "-c", resolved).CombinedOutput(); err != nil {
		t.Skipf("cannot start a private tmux server: %v (%s)", err, out)
	}
	t.Cleanup(func() { _ = exec.Command(bin, "kill-server").Run() })

	c := Client{R: LocalRunner{Bin: bin}}
	idx, err := c.NewWindow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(ctx, bin,
		"display-message", "-p", "-t", target(idx), "#{pane_current_path}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != resolved {
		t.Fatalf("new window cwd: got %q, want %q", got, resolved)
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
	t.Setenv("TMUX_TMPDIR", privateSocketDir(t))
	t.Cleanup(func() { _ = exec.Command(bin, "kill-server").Run() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Same argv the PTY channel uses, minus the attach (-d keeps it headless).
	args := append([]string{"new-session", "-d"}, AttachArgs("")[1:]...)
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
	t.Setenv("TMUX_TMPDIR", privateSocketDir(t))
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	t.Cleanup(func() { _ = exec.Command(bin, "kill-server").Run() })

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
