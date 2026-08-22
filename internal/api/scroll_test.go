package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestScrollEntersCopyModeThenScrolls(t *testing.T) {
	r := &fakeRunner{}
	rec := do(t, fakeServer(t, r), http.MethodPost, "/api/scroll", map[string]int{"lines": 4})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got []string
	for _, call := range r.got {
		got = append(got, strings.Join(call, " "))
	}
	want := []string{
		"copy-mode -e -t homebase",
		"send-keys -t homebase -X -N 4 scroll-up",
		"display-message -p -t homebase #{pane_in_mode}",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScrollNegativeGoesForward(t *testing.T) {
	r := &fakeRunner{}
	do(t, fakeServer(t, r), http.MethodPost, "/api/scroll", map[string]int{"lines": -2})
	if got := strings.Join(r.got[1], " "); got != "send-keys -t homebase -X -N 2 scroll-down" {
		t.Errorf("got %q", got)
	}
}

// lines:0 is the way back to the live shell for a client that tapped rather
// than swiping down to the bottom.
func TestScrollZeroOnlyCancels(t *testing.T) {
	r := &fakeRunner{}
	rec := do(t, fakeServer(t, r), http.MethodPost, "/api/scroll", map[string]int{"lines": 0})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(r.got) != 1 || strings.Join(r.got[0], " ") != "send-keys -t homebase -X cancel" {
		t.Fatalf("got %v", r.got)
	}
}
