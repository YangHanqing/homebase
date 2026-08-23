package web

import (
	"strings"
	"testing"
)

// A program that turned mouse reporting on (Claude Code sends DECSET 1000 +
// 1006) keeps its own scrollback and scrolls it on a wheel notch. Sending a
// swipe to tmux copy-mode there moves the wrong thing -- usually nothing at
// all, since the alternate screen has no history -- so the gesture has to
// reach the program instead. Both the finger and the wheel go through the
// same test: whoever is in the foreground decides.
func TestScrollFollowsTheForegroundProgramsMouseMode(t *testing.T) {
	term := readWeb(t, "js/terminal.js")
	for _, needle := range []string{
		"function homebaseMouseReporting(",
		"mouseTrackingMode",
		`"none"`,
		"new WheelEvent(",
		"deltaMode: 1",
	} {
		if !strings.Contains(term, needle) {
			t.Errorf("terminal.js missing %q", needle)
		}
	}

	js := readWeb(t, "js/app.js")
	body := between(t, js, "function scrollBy(", "\n  }")
	if !strings.Contains(body, "homebaseMouseReporting(term)") ||
		!strings.Contains(body, "homebaseSendWheel(") ||
		!strings.Contains(body, "queueScroll(lines)") {
		t.Errorf("scrollBy must route by mouse mode, got:\n%s", body)
	}
	if !strings.Contains(js, "scrollBy(lines, touch.clientX, touch.clientY)") {
		t.Error("the touch drag must go through scrollBy, not straight to tmux")
	}
}

// xterm's own wheel fallback for a buffer with no scrollback -- which is
// every buffer here, we are on tmux's alternate screen -- is to send one
// arrow key per line. Unstopped, a wheel over a shell walks its history
// instead of scrolling. This is verified against the vendored bundle so a
// re-vendor that changes the fallback is caught here rather than in the
// user's shell history.
func TestWheelIsTakenFromXtermWhenTheProgramDoesNotWantIt(t *testing.T) {
	if !strings.Contains(readWeb(t, "vendor/xterm/xterm.js"), "if(!this.buffer.hasScrollback)") {
		t.Error("xterm's no-scrollback wheel fallback moved; recheck what it sends now")
	}
	js := readWeb(t, "js/app.js")
	h := between(t, js, `termWrap.addEventListener("wheel"`, "passive: false })")
	for _, needle := range []string{
		"homebaseMouseReporting(term)", // the program's event: leave it to xterm
		"ev.stopPropagation()",
		"ev.preventDefault()",
		"queueScroll(lines)",
	} {
		if !strings.Contains(h, needle) {
			t.Errorf("wheel handler missing %q", needle)
		}
	}
	if !strings.Contains(h, "capture: true") {
		t.Error("the wheel handler must run ahead of xterm's own")
	}
}

// Keys typed into copy mode are eaten as copy-mode commands. A tap cancels
// it on the phone, but a wheel user just starts typing -- and the cancel
// travels on the control channel while the input travels on the WebSocket,
// with nothing ordering the two.
func TestTypingWaitsForTheCopyModeCancel(t *testing.T) {
	js := readWeb(t, "js/app.js")
	if strings.Count(js, "session.sendInput(") != 1 {
		t.Error("every input path must go through sendInput(), which owns the ordering")
	}
	body := between(t, js, "function afterCopyMode(", "\n  }")
	if !strings.Contains(body, "pendingInput = leaveCopyMode()") ||
		!strings.Contains(body, "pendingInput.then(fn)") {
		t.Errorf("afterCopyMode must queue behind the cancel, got:\n%s", body)
	}
}

func between(t *testing.T, s, start, end string) string {
	t.Helper()
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatalf("%q not found", start)
	}
	rest := s[i:]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("%q not found after %q", end, start)
	}
	return rest[:j]
}
