package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeTmux writes a script that records its argv to argfile and exits, standing
// in for the real tmux binary via LocalDialer.Tmux.
func fakeTmux(t *testing.T) (bin, argfile string) {
	t.Helper()
	dir := t.TempDir()
	argfile = filepath.Join(dir, "args")
	bin = filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argfile + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argfile
}

func startAndRead(t *testing.T, d LocalDialer, argfile string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := d.Start(ctx, Size{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Kill()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(argfile); err == nil && len(b) > 0 {
			return string(b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fake tmux never recorded its argv")
	return ""
}

// A project dialer's StartDir must reach tmux as the session's -c, so the
// session created by the first post-reboot attach lands in the project.
func TestStartUsesStartDirForNewSession(t *testing.T) {
	bin, argfile := fakeTmux(t)
	got := startAndRead(t, LocalDialer{Tmux: bin, Session: "homebase-abc", StartDir: "/tmp/proj"}, argfile)
	if !strings.Contains(got, "\n-c\n/tmp/proj\n") {
		t.Fatalf("expected -c /tmp/proj in argv, got:\n%s", got)
	}
}

// An empty StartDir (the legacy singleton) still falls back to $HOME.
func TestStartFallsBackToHome(t *testing.T) {
	bin, argfile := fakeTmux(t)
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home dir")
	}
	got := startAndRead(t, LocalDialer{Tmux: bin}, argfile)
	if !strings.Contains(got, "\n-c\n"+home+"\n") {
		t.Fatalf("expected -c %s in argv, got:\n%s", home, got)
	}
}
