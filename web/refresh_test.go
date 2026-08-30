package web

import (
	"strings"
	"testing"
)

// The sidebar's list is not just decoration: newWindow reads `sections` to
// decide attach-only vs. POST, and killWindow reads it to decide whether the
// session just ended. A refresh dropped because the 3s poll happened to be in
// flight leaves both reading a pre-action list, because that poll's requests
// went out *before* the action -- which is how a single "+" click created two
// tmux windows. Queue the refresh; never discard it.
func TestRefreshWindowsNeverDropsAMidFlightRefresh(t *testing.T) {
	body := refreshWindowsBody(t)

	if !strings.Contains(body, "inFlight") {
		t.Error("refreshWindows must still know a poll is in flight; two concurrent fetch fans would interleave their writes to sections")
	}
	if !strings.Contains(body, "refreshWaiters") {
		t.Error("a refresh that collides with an in-flight one must be queued in refreshWaiters, not dropped")
	}
	// The exact shape of the old discard. If it comes back, the caller is
	// handed a promise that resolves before its own refresh ever ran.
	if strings.Contains(body, "return Promise.resolve();") {
		t.Error("refreshWindows discards a mid-flight refresh again; the action's list write is lost and sections goes stale")
	}
}

// Queueing a refresh is only half the fix -- the queue has to drain. If the
// finally block forgets to re-run, an action's refresh is deferred forever and
// its caller's promise never settles, so killWindow's "was that the last
// window" branch simply never runs.
func TestRefreshWindowsRerunsTheQueuedRefresh(t *testing.T) {
	js := readWeb(t, "js/app.js")

	// The array has to outlive one call, or a waiter recorded during a poll is
	// invisible to the poll that must re-run it.
	decl := strings.Index(js, "let refreshWaiters")
	if decl < 0 {
		t.Fatal("refreshWaiters must be a module-level array; a per-call one cannot carry a waiter across the collision")
	}
	if fn := strings.Index(js, "function refreshWindows()"); fn >= 0 && decl > fn {
		t.Error("refreshWaiters must be declared outside refreshWindows so it survives between calls")
	}

	body := refreshWindowsBody(t)
	i := strings.Index(body, "finally")
	if i < 0 {
		t.Fatal("refreshWindows must clear inFlight in a finally; a rejected poll would otherwise wedge it on forever")
	}
	tail := body[i:]
	if !strings.Contains(tail, "refreshWaiters") {
		t.Error("the finally block must drain refreshWaiters when the current poll ends")
	}
	if !strings.Contains(tail, "refreshWindows()") {
		t.Error("the finally block must re-run refreshWindows for the queued waiters; queueing without a re-run is a hang")
	}
	if !strings.Contains(tail, "resolve") {
		t.Error("each waiter must be resolved with the re-run, or the action that requested it never learns the list is current")
	}
}

// Typing `exit` in a project's last window ends its tmux session while that
// project stays "current" and the socket stays down until the backoff is due.
// ensureConnected is a no-op for the current project, so "+" attached nothing
// and returned: the button was simply dead. Waking the socket is what recreates
// the session, and its attach brings the first window with it.
func TestPlusOnAnEmptyAttachedProjectWakesTheSocket(t *testing.T) {
	body := newWindowBody(t)

	if !strings.Contains(body, "currentProject") {
		t.Error("newWindow must distinguish an empty project that is already attached from one that is not; ensureConnected does nothing for the former")
	}
	if !strings.Contains(body, "session.wake()") {
		t.Error("an empty project that is already the attached one must wake the socket; nothing else recreates the session before the backoff is due")
	}
	if !strings.Contains(body, `lastStatus.state !== "connected"`) {
		t.Error("the wake branch must be gated on the socket actually being down; waking a live connection is not what an empty-but-connected list means")
	}
}

// Both attach paths create the window through `new-session -A -c <path>`.
// A POST alongside either one races that attach and lands two windows where
// the user asked for one, so each branch has to leave before the POST is
// reached. (projects_test.go covers the attach-only branch itself.)
func TestNewWindowExitsBothAttachBranchesBeforePosting(t *testing.T) {
	body := newWindowBody(t)

	post := strings.Index(body, `method: "POST"`)
	if post < 0 {
		t.Fatal("newWindow must still POST /api/windows for a project that already has windows")
	}
	attach := strings.Index(body, "ensureConnected(project)")
	if attach < 0 || attach > post {
		t.Error("the attach-only branch must run before the POST is reached, and return there")
	}
	wake := strings.Index(body, "session.wake()")
	if wake < 0 || wake > post {
		t.Error("the wake branch must run before the POST is reached, and return there")
	}
	if n := strings.Count(body[:post], "return;"); n < 2 {
		t.Errorf("both attach branches must return before the POST; found %d early returns, want 2", n)
	}
}

// Closing a project's last window ends its tmux session on purpose (AGENT.md
// hard constraint 3). One second later the WebSocket's backoff fires
// `new-session -A` and the session the user just closed is back, with a fresh
// window in it. A reconnect that ignores the hold is that whole bug.
func TestSessionCanHoldOffTheReconnect(t *testing.T) {
	js := readWeb(t, "js/session.js")

	if !strings.Contains(js, "HomebaseSession.prototype.hold") {
		t.Error("HomebaseSession must expose hold() so a control action that may end the session can suspend the backoff loop")
	}
	if !strings.Contains(js, "HomebaseSession.prototype.release") {
		t.Error("HomebaseSession must expose release(), or a held session stays disconnected for the life of the page")
	}

	sched := between(t, js, "HomebaseSession.prototype.scheduleReconnect", "\n};")
	if !strings.Contains(sched, "this.held") {
		t.Error("scheduleReconnect must return early while held; otherwise the backoff resurrects the session the user just closed")
	}
	wake := between(t, js, "HomebaseSession.prototype.wake", "\n};")
	if !strings.Contains(wake, "this.held") {
		t.Error("wake() must respect the hold too; it is the same reconnect by another door")
	}

	hold := between(t, js, "HomebaseSession.prototype.hold", "\n};")
	if !strings.Contains(hold, "this.held = true") {
		t.Error("hold() must set the flag it is named for")
	}
	if !strings.Contains(hold, "clearTimer") {
		t.Error("hold() must cancel the reconnect already scheduled; a pending timer fires regardless of the flag set after it")
	}
}

// The other half: a hold that is never lifted leaves a working project stuck
// on a dead socket, showing nothing, with no retry ever scheduled -- the
// backoff loop is exactly what was suppressed.
func TestSessionReleaseReconnectsIfTheSocketDiedWhileHeld(t *testing.T) {
	rel := between(t, readWeb(t, "js/session.js"), "HomebaseSession.prototype.release", "\n};")

	if !strings.Contains(rel, "this.held = false") {
		t.Error("release() must clear the hold, or every later close is silently swallowed")
	}
	if !strings.Contains(rel, "this.connect()") {
		t.Error("release() must reconnect when the socket died while held; nothing else is going to, since the backoff was suppressed")
	}
}

// The hold has to be in place before the DELETE goes out, not after it: the
// close and the socket's death are the same event, and a backoff scheduled in
// between is what brings the session back.
func TestKillWindowHoldsTheSocketBeforeTheDelete(t *testing.T) {
	body := killWindowBody(t)

	hold := strings.Index(body, "session.hold()")
	if hold < 0 {
		t.Fatal("killWindow must hold the attached session's reconnect while the DELETE is in flight")
	}
	del := strings.Index(body, `method: "DELETE"`)
	if del < 0 {
		t.Fatal("killWindow must still DELETE /api/windows/{index}")
	}
	if hold > del {
		t.Error("the hold must be taken before the DELETE; a reconnect scheduled in the gap recreates the session the DELETE just ended")
	}
	if !strings.Contains(body, "currentProject") {
		t.Error("only the attached project's session may be held; holding while killing a window elsewhere would freeze the terminal for no reason")
	}
}

// Counting the windows before the request answers with a list that may be a
// poll behind. Get it wrong one way and the UI stays attached to a session
// that no longer exists; wrong the other way and it drops a live project back
// to Ungrouped. The refreshed list is the only trustworthy count.
func TestKillWindowDecidesFromTheRefreshedList(t *testing.T) {
	body := killWindowBody(t)

	if strings.Contains(body, "killingLast") {
		t.Error("killWindow decides from a pre-action count again; that list can be a poll behind, and the answer flips the wrong way")
	}
	if !strings.Contains(body, "sections[project]") || !strings.Contains(body, "windows.length") {
		t.Error("killWindow must read the refreshed window count for that project after the DELETE settles")
	}

	del := strings.Index(body, `method: "DELETE"`)
	decide := strings.Index(body, "sections[project]")
	if del < 0 || decide < 0 || decide < del {
		t.Error("the last-window decision must be made after the DELETE, from the list the action's own refresh produced")
	}

	if !strings.Contains(body, `connect("")`) {
		t.Error("an emptied project session is gone; the terminal must fall back to the legacy singleton session")
	}
	if !strings.Contains(body, "session.release()") {
		t.Error("a project that still has windows must have its reconnect released, or it never comes back after a transient close")
	}
}

func refreshWindowsBody(t *testing.T) string {
	t.Helper()
	return between(t, readWeb(t, "js/app.js"), "function refreshWindows()", "\n  function refreshAll")
}

func newWindowBody(t *testing.T) string {
	t.Helper()
	return between(t, readWeb(t, "js/app.js"), "function newWindow(project) {", "\n  function killWindow")
}

func killWindowBody(t *testing.T) string {
	t.Helper()
	return between(t, readWeb(t, "js/app.js"), "function killWindow(project, index) {", "\n  // ---- projects")
}
