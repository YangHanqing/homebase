package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/yanghanqing/homebase/internal/projects"
	"github.com/yanghanqing/homebase/internal/tmux"
)

func testProjects(t *testing.T) *projects.Store {
	t.Helper()
	s, err := projects.Load(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func projServer(t *testing.T, r *fakeRunner, ps *projects.Store) http.Handler {
	t.Helper()
	return NewMuxServer(&Server{
		Store:    testStore(t),
		Projects: ps,
		NewTmux:  func() tmux.Client { return tmux.Client{R: r} },
	})
}

// Deleting a project is explicit intent to end it, so it must also kill that
// project's tmux session (never any other session) -- not just drop the
// projects.json entry.
func TestProjectDeleteKillsItsSession(t *testing.T) {
	ps := testProjects(t)
	dir := t.TempDir()
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodPost, "/api/projects", map[string]string{"path": dir})
	p := decode[projects.Project](t, rec)

	r := &fakeRunner{}
	h2 := projServer(t, r, ps)
	rec = do(t, h2, http.MethodDelete, "/api/projects/"+p.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var sawKill bool
	for _, args := range r.got {
		if args[0] == "kill-session" {
			sawKill = true
			if want := tmux.ProjectSession(p.ID); args[2] != want {
				t.Fatalf("kill-session target = %q, want %q", args[2], want)
			}
		}
	}
	if !sawKill {
		t.Fatalf("expected kill-session, got %v", r.got)
	}
}

// A session that is already gone must not turn a completed deletion into an
// error response -- the projects.json entry is removed either way.
func TestProjectDeleteSucceedsWhenSessionAlreadyGone(t *testing.T) {
	ps := testProjects(t)
	dir := t.TempDir()
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodPost, "/api/projects", map[string]string{"path": dir})
	p := decode[projects.Project](t, rec)

	r := &fakeRunner{err: tmux.ErrNoSession}
	h2 := projServer(t, r, ps)
	rec = do(t, h2, http.MethodDelete, "/api/projects/"+p.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectAddListRemove(t *testing.T) {
	ps := testProjects(t)
	h := projServer(t, &fakeRunner{}, ps)
	dir := t.TempDir()

	rec := do(t, h, http.MethodPost, "/api/projects", map[string]string{"path": dir})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	p := decode[projects.Project](t, rec)
	if p.Path != dir || p.ID == "" {
		t.Fatalf("got %+v", p)
	}

	rec = do(t, h, http.MethodGet, "/api/projects", nil)
	list := decode[struct {
		Projects []projects.Project `json:"projects"`
	}](t, rec)
	if len(list.Projects) != 1 || list.Projects[0].ID != p.ID {
		t.Fatalf("got %+v", list)
	}

	rec = do(t, h, http.MethodDelete, "/api/projects/"+p.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, http.MethodDelete, "/api/projects/"+p.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: status %d", rec.Code)
	}
}

func TestProjectAddRejectsNonDirectory(t *testing.T) {
	ps := testProjects(t)
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodPost, "/api/projects", map[string]string{"path": "relative"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectRouteRejectsBadID(t *testing.T) {
	ps := testProjects(t)
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodDelete, "/api/projects/not-hex", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

// A window route naming an untracked (or malformed) project must not fall
// back to the legacy singleton session -- that would let the browser talk to
// a session it never asked to track.
func TestWindowsRouteRejectsUnknownProject(t *testing.T) {
	ps := testProjects(t)
	r := &fakeRunner{list: "0 1 1787485591 0 shell\n"}
	h := projServer(t, r, ps)
	rec := do(t, h, http.MethodGet, "/api/windows?project=000000000000", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(r.got) != 0 {
		t.Fatalf("must not reach tmux for an unknown project: %v", r.got)
	}
}

func TestWindowsRouteScopesToProjectSession(t *testing.T) {
	ps := testProjects(t)
	dir := t.TempDir()
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodPost, "/api/projects", map[string]string{"path": dir})
	p := decode[projects.Project](t, rec)

	r := &fakeRunner{list: "0 1 1787485591 0 shell\n"}
	h2 := projServer(t, r, ps)
	rec = do(t, h2, http.MethodGet, "/api/windows?project="+p.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	for _, args := range r.got {
		if args[0] == "list-windows" {
			joined := args[2]
			if joined != tmux.ProjectSession(p.ID) {
				t.Fatalf("list-windows target = %q, want %q", joined, tmux.ProjectSession(p.ID))
			}
		}
	}
}

func TestBrowseDefaultsToHome(t *testing.T) {
	ps := testProjects(t)
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodGet, "/api/browse", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBrowseRejectsRelativePath(t *testing.T) {
	ps := testProjects(t)
	h := projServer(t, &fakeRunner{}, ps)
	rec := do(t, h, http.MethodGet, "/api/browse?path=relative", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}
