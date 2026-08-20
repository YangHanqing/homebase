package session

import (
	"os"
	"strings"
	"testing"

	"github.com/yanghanqing/homebase/internal/tmux"
)

func TestExecEnvSetsTERM(t *testing.T) {
	env := tmux.ExecEnv()
	found := false
	for _, e := range env {
		if e == "TERM=xterm-256color" {
			found = true
		}
		if strings.HasPrefix(e, "TERM=") && e != "TERM=xterm-256color" {
			t.Fatalf("TERM should be forced, got %q", e)
		}
	}
	if !found {
		t.Fatal("missing TERM=xterm-256color")
	}
	if os.Getenv("HOME") == "" {
		hasHome := false
		for _, e := range env {
			if strings.HasPrefix(e, "HOME=") && len(e) > 5 {
				hasHome = true
			}
		}
		if !hasHome {
			t.Fatal("HOME should be filled when unset")
		}
	}
}
