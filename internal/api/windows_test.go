package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/yanghanqing/homebase/internal/tmux"
)

// fakeRunner replays canned tmux output and records the argv it was handed,
// so the tests assert on the control channel without a tmux server.
type fakeRunner struct {
	list string
	err  error
	got  [][]string
}

func (f *fakeRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	f.got = append(f.got, args)
	if f.err != nil {
		return nil, f.err
	}
	if len(args) > 0 && args[0] == "list-windows" {
		return []byte(f.list), nil
	}
	if len(args) > 0 && args[0] == "new-window" {
		return []byte("3\n"), nil
	}
	return nil, nil
}

func fakeServer(t *testing.T, r *fakeRunner) http.Handler {
	t.Helper()
	return NewMuxServer(&Server{
		Store:   testStore(t),
		NewTmux: func() tmux.Client { return tmux.Client{R: r} },
	})
}

func TestListWindows(t *testing.T) {
	r := &fakeRunner{list: "0 1 shell\n1 0 logs\n"}
	rec := do(t, fakeServer(t, r), http.MethodGet, "/api/windows", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[struct {
		Windows []tmux.Window `json:"windows"`
	}](t, rec)
	if len(body.Windows) != 2 {
		t.Fatalf("got %+v", body.Windows)
	}
	if body.Windows[0].Name != "shell" || !body.Windows[0].Active {
		t.Fatalf("first window %+v", body.Windows[0])
	}
	if body.Windows[1].Index != 1 || body.Windows[1].Active {
		t.Fatalf("second window %+v", body.Windows[1])
	}
}

func TestNewWindow(t *testing.T) {
	r := &fakeRunner{}
	rec := do(t, fakeServer(t, r), http.MethodPost, "/api/windows", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]int](t, rec); got["index"] != 3 {
		t.Fatalf("index %v", got)
	}
}

func TestRenameAndSelectWindow(t *testing.T) {
	r := &fakeRunner{}
	rec := do(t, fakeServer(t, r), http.MethodPut, "/api/windows/2",
		map[string]any{"name": "build", "active": true})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var sawRename, sawSelect bool
	for _, args := range r.got {
		switch args[0] {
		case "rename-window":
			sawRename = true
			if !strings.HasSuffix(strings.Join(args, " "), "build") {
				t.Fatalf("rename argv %v", args)
			}
		case "select-window":
			sawSelect = true
		}
	}
	if !sawRename || !sawSelect {
		t.Fatalf("argv %v", r.got)
	}
}

func TestRenameRejectsBadName(t *testing.T) {
	for _, name := range []string{"bad\x07name", ";", strings.Repeat("x", 200)} {
		r := &fakeRunner{}
		rec := do(t, fakeServer(t, r), http.MethodPut, "/api/windows/0",
			map[string]any{"name": name})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status %d: %s", name, rec.Code, rec.Body.String())
		}
		if len(r.got) != 0 {
			t.Errorf("%q reached tmux: %v", name, r.got)
		}
	}
}

func TestWindowIndexMustBeValid(t *testing.T) {
	r := &fakeRunner{}
	for _, path := range []string{
		"/api/windows/-1",
		"/api/windows/abc",
	} {
		rec := do(t, fakeServer(t, r), http.MethodDelete, path, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d", path, rec.Code)
		}
	}
}

func TestKillWindow(t *testing.T) {
	r := &fakeRunner{list: "0 1 shell\n1 0 logs\n"}
	rec := do(t, fakeServer(t, r), http.MethodDelete, "/api/windows/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestKillLastWindowIsRefused(t *testing.T) {
	r := &fakeRunner{list: "0 1 shell\n"}
	rec := do(t, fakeServer(t, r), http.MethodDelete, "/api/windows/0", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec); got["code"] != "last_window" {
		t.Fatalf("code %v", got)
	}
	for _, args := range r.got {
		if args[0] == "kill-window" {
			t.Fatalf("kill-window ran anyway: %v", r.got)
		}
	}
}

func TestMissingTmuxIsBadGateway(t *testing.T) {
	r := &fakeRunner{err: tmux.ErrNoTmux}
	rec := do(t, fakeServer(t, r), http.MethodGet, "/api/windows", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec); got["code"] != "enotmux" {
		t.Fatalf("code %v", got)
	}
}
