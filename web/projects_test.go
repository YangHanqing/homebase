package web

import (
	"strings"
	"testing"
)

// Adding a project must attach to it so new-session -A -c <path> creates the
// first window in that folder. Also POSTing /api/windows races the attach and
// often produces a second window.
func TestAddingAProjectOpensTheFirstWindowByAttaching(t *testing.T) {
	js := readWeb(t, "js/app.js")
	body := between(t, js, `projectForm.addEventListener("submit"`, "// ---- directory picker")
	if !strings.Contains(body, "ensureConnected(p.id)") {
		t.Error("adding a project must attach so the first window lands in the project directory")
	}
	if strings.Contains(body, "/api/windows") {
		t.Error("must not also POST /api/windows; that races the attach and can create two windows")
	}
}

// "+" on a project that currently has no windows is the same attach-only
// path: the PTY connect creates the one window. POST /api/windows stays for
// projects that already have a session.
func TestNewWindowOnEmptyProjectOnlyAttaches(t *testing.T) {
	js := readWeb(t, "js/app.js")
	body := between(t, js, "function newWindow(project) {", "\n  function killWindow")
	if !strings.Contains(body, "!s.windows.length") {
		t.Error("newWindow must detect an empty project")
	}
	// The branch is conditioned on more than "empty" now: an empty project
	// that is already the attached one needs a reconnect rather than an
	// attach, because ensureConnected is a no-op there. So this pins the
	// shape that matters rather than the exact condition — emptiness is one
	// of the operands, and that path returns instead of falling through to
	// the POST. web/refresh_test.go pins the branches themselves.
	if !strings.Contains(body, "if (empty &&") || !strings.Contains(body, "return;") {
		t.Error("an empty project must return after attach, not POST /api/windows")
	}
	if !strings.Contains(body, "ensureConnected(project)") {
		t.Error("newWindow must attach so the first window is created with -c <path>")
	}
	if !strings.Contains(body, `method: "POST"`) {
		t.Error("newWindow must still POST /api/windows when the project already has windows")
	}
}

// Ungrouped ("") is bound by the attach-only rule too. Its singleton session
// ends when its last window exits, and nothing recreates it while the one
// terminal is attached to a project instead — so "+" on Ungrouped would
// attach (creating the session's first window) *and* POST /api/windows
// (whose new-session fallback then loses the "duplicate session" race and
// adds a second one). Gating "empty" on the project id being non-empty is
// exactly that bug, so pin its absence.
func TestNewWindowTreatsUngroupedLikeAProject(t *testing.T) {
	js := readWeb(t, "js/app.js")
	body := between(t, js, "function newWindow(project) {", "\n  function killWindow")
	if strings.Contains(body, "!!project &&") {
		t.Error(`"empty" must not be gated on a non-empty project id: Ungrouped needs the same attach-only path`)
	}
	if !strings.Contains(body, "const empty = !s || !s.windows.length;") {
		t.Error("emptiness must be read from the section's window list alone")
	}
}
