package web

import (
	"os"
	"strings"
	"testing"
)

func TestSessionKeepsClassifiedErrorOnClose(t *testing.T) {
	src, err := os.ReadFile("js/session.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `self.state !== "error"`) {
		t.Fatal("ws onclose must not overwrite a classified error with disconnected")
	}
}

func TestBackoffSequence(t *testing.T) {
	src, err := os.ReadFile("js/backoff.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "Math.min(30000, 1000 * Math.pow(2, attempt))") {
		t.Fatal("backoff.js must keep the documented formula")
	}
	want := []int{1000, 2000, 4000, 8000, 16000, 30000, 30000}
	for attempt, w := range want {
		got := nextDelayMs(attempt)
		if got != w {
			t.Errorf("attempt %d: got %d want %d", attempt, got, w)
		}
	}
}

func nextDelayMs(attempt int) int {
	d := 1000
	for i := 0; i < attempt; i++ {
		if d >= 30000 {
			return 30000
		}
		d *= 2
	}
	if d > 30000 {
		return 30000
	}
	return d
}
