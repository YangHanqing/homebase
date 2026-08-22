package web

import (
	"os"
	"strings"
	"testing"
)

// The on-screen toolbar is the only way a phone can send these, and it has
// room for exactly seven buttons at Apple's 44pt minimum on a 320pt screen.
// Anything added here has to displace something, not squeeze the row.
func TestKeyToolbarStaysSevenKeys(t *testing.T) {
	s := readWeb(t, "index.html")
	start := strings.Index(s, `id="key-toolbar"`)
	if start < 0 {
		t.Fatal("key toolbar missing")
	}
	bar := s[start:]
	bar = bar[:strings.Index(bar, "</div>")]
	if n := strings.Count(bar, "<button"); n != 7 {
		t.Errorf("toolbar has %d buttons, want 7", n)
	}
	for _, needle := range []string{
		`data-mod="ctrl"`, // sticky Ctrl: the only way to reach an arbitrary ^X
		`data-key="Escape"`,
		`data-key="Tab"`,
		`data-key="ArrowUp"`,
		`data-key="ArrowDown"`,
		`data-key="C-c"`, // one tap, because interrupting often happens with
		`id="key-more"`,  // the on-screen keyboard down and Ctrl unusable
	} {
		if !strings.Contains(bar, needle) {
			t.Errorf("toolbar missing %s", needle)
		}
	}
}

// Every button in the panel must resolve to something app.js can send.
func TestPanelKeysAllResolve(t *testing.T) {
	html := readWeb(t, "index.html")
	js := readWeb(t, "js/app.js")
	seqStart := strings.Index(js, "const KEY_SEQ = {")
	if seqStart < 0 {
		t.Fatal("KEY_SEQ table missing")
	}
	table := js[seqStart : strings.Index(js[seqStart:], "};")+seqStart]

	for _, part := range strings.Split(html, `data-key="`)[1:] {
		name := part[:strings.Index(part, `"`)]
		if strings.HasPrefix(name, "C-") && len(name) == 3 {
			continue // derived from the control byte, not the table
		}
		if !strings.Contains(table, name+":") && !strings.Contains(table, `"`+name+`":`) {
			t.Errorf("data-key=%q has no sequence in KEY_SEQ", name)
		}
	}
}

// Double-tap sends Tab. Without preventDefault, Safari synthesises the mouse
// events behind it, xterm selects a word, and copy-on-select silently
// overwrites the clipboard on every completion.
func TestDoubleTapSuppressesSynthesisedMouseEvents(t *testing.T) {
	js := readWeb(t, "js/app.js")
	i := strings.Index(js, `termWrap.addEventListener("touchend"`)
	if i < 0 {
		t.Fatal("double-tap handler missing")
	}
	h := js[i:]
	if !strings.Contains(h, "ev.preventDefault()") {
		t.Error("double-tap must preventDefault to stop the synthesised double-click")
	}
	if !strings.Contains(h, "passive: false") {
		t.Error("a passive listener cannot preventDefault")
	}
	if !strings.Contains(h, `get("doubleTapTab")`) {
		t.Error("the gesture must honour its preference")
	}
}

// xterm measures its scroll bar once, as (viewport - scrollArea) || 15, so on
// every touch platform -- where overlay scroll bars make that difference 0 --
// FitAddon deducts a phantom 15px, about two columns on a phone, forever.
// Hiding the scroll bar in CSS makes the difference 0 and so *causes* the
// fallback; the reclaim has to happen after the fit instead.
func TestFitReclaimsThePhantomScrollBar(t *testing.T) {
	js := readWeb(t, "js/terminal.js")
	if !strings.Contains(js, "homebaseReclaimGutter(term)") {
		t.Error("homebaseFitSize must reclaim the gutter after fitting")
	}
	for _, needle := range []string{"term.resize(cols, rows)", "wrap.clientWidth", "term.cols"} {
		if !strings.Contains(js, needle) {
			t.Errorf("gutter reclaim missing %q", needle)
		}
	}
	css := readWeb(t, "css/app.css")
	if strings.Contains(css, "scrollbar-width: none") {
		t.Error("hiding the scroll bar re-triggers xterm's 15px fallback")
	}
	if !strings.Contains(css, "touch-action: manipulation;\n}") {
		t.Error(".term-wrap needs touch-action: manipulation or Safari zooms on double-tap")
	}
}

// Both the scroll gesture and long-press selection depend on beating xterm to
// the touch events. xterm's touchmove listener is
// `handleTouchMove(e) ? undefined : cancel(e)`, and handleTouchMove always
// returns false here -- the viewport has no scrollback because we are on
// tmux's alternate screen -- so xterm preventDefaults every touchmove over
// the terminal unless we capture first and stop propagation.
func TestTouchHandlersCaptureAheadOfXterm(t *testing.T) {
	js := readWeb(t, "js/app.js")
	for _, ev := range []string{"touchstart", "touchmove", "touchend"} {
		i := strings.Index(js, `termWrap.addEventListener("`+ev+`"`)
		if i < 0 {
			t.Fatalf("%s handler missing", ev)
		}
		end := strings.Index(js[i:], "});")
		if end < 0 || !strings.Contains(js[i:i+end+3], "capture: true") {
			t.Errorf("%s handler must run in the capture phase", ev)
		}
	}
	move := js[strings.Index(js, `termWrap.addEventListener("touchmove"`):]
	move = move[:strings.Index(move, "passive: false })")]
	if !strings.Contains(move, "ev.stopPropagation()") {
		t.Error("touchmove must stop propagation or xterm cancels the gesture")
	}
	// The selection branch must NOT preventDefault: that is what hands the
	// drag to iOS's own magnifier and Copy callout.
	sel := move[strings.Index(move, `gesture.mode === "select"`):]
	sel = sel[:strings.Index(sel, "return;")]
	if strings.Contains(sel, "ev.preventDefault()") {
		t.Error("the selection branch must leave the default action to iOS")
	}

	css := readWeb(t, "css/app.css")
	if !strings.Contains(css, ".term-wrap .xterm-rows") || !strings.Contains(css, "user-select: text") {
		t.Error("xterm sets user-select:none; the rows must opt back in for long-press selection")
	}
}

// A swipe is a REST call, so a fast flick must not queue a burst of them.
func TestScrollRequestsAreSerialised(t *testing.T) {
	js := readWeb(t, "js/app.js")
	for _, needle := range []string{"scrollInFlight", "pendingLines += lines", "/api/scroll"} {
		if !strings.Contains(js, needle) {
			t.Errorf("scroll queue missing %q", needle)
		}
	}
}

func readWeb(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
