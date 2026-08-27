package web

import (
	"strings"
	"testing"
)

// The premise of the guard, verified against the vendored bundle: while
// xterm believes a composition is in progress it drops every keydown whose
// keyCode is 229 -- which is what a phone keyboard reports for every key. So
// a single missing `compositionend` (switching the iOS keyboard from Chinese
// to English produces one) means nothing can be typed for the life of the
// page. If a re-vendor changes this, the guard's reason to exist changed too.
func TestXtermSwallowsPhoneKeysWhileItThinksItIsComposing(t *testing.T) {
	x := readWeb(t, "vendor/xterm/xterm.js")
	kd := between(t, x, "keydown(e){if(this._isComposing", "_finalizeComposition")
	if !strings.Contains(kd, "229===e.keyCode") {
		t.Error("xterm's composition keydown gate moved; recheck why input is swallowed")
	}
	// Nothing public clears that flag, which is why the guard has to send the
	// missing event rather than call an API.
	if strings.Contains(x, "resetComposition") {
		t.Error("xterm may have grown a way to reset composition; prefer it to a synthetic event")
	}
}

func TestIMEGuardCannotGoStaleOnTheMissingEvent(t *testing.T) {
	js := readWeb(t, "js/terminal.js")
	body := between(t, js, "function homebaseGuardIME(", "\n}")

	// A boolean set by compositionstart and cleared by compositionend would go
	// stale on exactly the event that is missing. The evidence has to be a
	// timestamp: a live composition emits an event per keystroke.
	if !strings.Contains(body, "lastComposition = Date.now()") ||
		!strings.Contains(body, "HOMEBASE_IME_LIVE_MS") {
		t.Error("the guard must judge a live composition by recency, not by a flag")
	}
	for _, ev := range []string{"compositionstart", "compositionupdate", "compositionend"} {
		if !strings.Contains(body, `"`+ev+`"`) {
			t.Errorf("guard ignores %s", ev)
		}
	}
	if !strings.Contains(body, "ev.isComposing") {
		t.Error("an input event that reports itself as composing is never stuck")
	}

	// Clearing the textarea before handing xterm the compositionend is what
	// makes the recovery safe: xterm's finalizer reads the textarea inside a
	// timeout, finds it empty, and sends nothing -- so the replay below cannot
	// arrive twice.
	clear := strings.Index(body, `ta.value = ""`)
	dispatch := strings.Index(body, "dispatchEvent(new Event(\"compositionend\")")
	if clear < 0 || dispatch < 0 || clear > dispatch {
		t.Error("the textarea must be cleared before the synthetic compositionend, or the replay doubles up")
	}
	if !strings.Contains(body, "HOMEBASE_IME_MAX_REPLAY") {
		t.Error("the replay must be bounded; the textarea is a running log, not one keystroke")
	}

	if !strings.Contains(readWeb(t, "js/app.js"), "homebaseGuardIME(term, typed)") {
		t.Error("the guard must replay through the same path as a live keystroke")
	}
}

// A paste is not a swallowed keystroke. xterm sends the clipboard from its own
// `paste` listener and calls stopPropagation but never preventDefault, so the
// browser still inserts the text into the textarea and fires `input` behind it
// -- with nothing following on the data channel, which is exactly the shape the
// guard replays. That made Cmd+V arrive twice for any paste short enough to
// clear the replay bound. If a re-vendor starts cancelling the default, this
// premise changed and the guard's paste handling can go.
func TestXtermLetsAPastedTextReachTheTextarea(t *testing.T) {
	x := readWeb(t, "vendor/xterm/xterm.js")
	h := between(t, x, "t.handlePasteEvent=function", "t.paste=")
	if !strings.Contains(h, "stopPropagation") {
		t.Error("xterm's paste handler moved; recheck how a paste reaches the textarea")
	}
	if strings.Contains(h, "preventDefault") {
		t.Error("xterm now cancels the paste insert; the guard's paste handling is stale")
	}
}

func TestIMEGuardDoesNotReplayAPaste(t *testing.T) {
	body := between(t, readWeb(t, "js/terminal.js"), "function homebaseGuardIME(", "\n}")

	if !strings.Contains(body, `ta.addEventListener("paste"`) || !strings.Contains(body, "ev.preventDefault()") {
		t.Error("the guard must cancel the textarea insert a paste would otherwise leave behind")
	}
	// And if some browser inserts it anyway, the pasted text must be accounted
	// for rather than replayed -- xterm already sent it.
	if !strings.Contains(body, `ev.inputType === "insertFromPaste"`) {
		t.Error("a paste-driven input event must never be treated as swallowed input")
	}
}
