// Which palette the terminal should draw with. "system" defers to the OS;
// "light"/"dark" are pinned in Settings and must win over it, exactly like the
// data-theme attribute does for the surrounding page chrome.
function homebaseWantsLight() {
  const prefs = window.homebasePrefs;
  const choice = prefs ? prefs.get("theme") : "system";
  if (choice === "light") {
    return true;
  }
  if (choice === "dark") {
    return false;
  }
  return !!(window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches);
}

function homebaseTermTheme() {
  if (homebaseWantsLight()) {
    return {
      background: "#f6f3ec",
      foreground: "#1a1916",
      cursor: "#b8860b",
      selectionBackground: "#d9d4c8",
      black: "#1a1916",
      red: "#b44532",
      green: "#2d6a50",
      yellow: "#b8860b",
      blue: "#3d5a6e",
      magenta: "#6e4e68",
      cyan: "#3d6a5e",
      white: "#1a1916"
    };
  }
  return {
    background: "#1a1916",
    foreground: "#e8e4db",
    cursor: "#d4a017",
    selectionBackground: "#3a3732",
    black: "#1a1916",
    red: "#b44532",
    green: "#3d8c6e",
    yellow: "#d4a017",
    blue: "#6e8b9a",
    magenta: "#8a6e84",
    cyan: "#6e8f82",
    white: "#e8e4db"
  };
}

function homebasePref(key, fallback) {
  const prefs = window.homebasePrefs;
  if (!prefs) {
    return fallback;
  }
  const v = prefs.get(key);
  return v === undefined ? fallback : v;
}

function homebaseCreateTerminal(container) {
  const term = new Terminal({
    cursorBlink: homebasePref("cursorBlink", true),
    fontFamily: "ui-monospace, Menlo, monospace",
    fontSize: homebasePref("fontSize", 13),
    lineHeight: 1.2,
    theme: homebaseTermTheme(),
    scrollback: homebasePref("scrollback", 4000),
    allowProposedApi: true
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  const unicode11 = new Unicode11Addon.Unicode11Addon();
  term.loadAddon(unicode11);
  term.unicode.activeVersion = "11";
  term.open(container);
  homebaseBindCopyOnSelect(term);

  // The system palette moving matters only while theme is "system"; the
  // preference event covers the pinned cases and everything else.
  if (window.matchMedia) {
    const mq = window.matchMedia("(prefers-color-scheme: light)");
    const onSystem = function () { term.options.theme = homebaseTermTheme(); };
    if (mq.addEventListener) {
      mq.addEventListener("change", onSystem);
    } else if (mq.addListener) {
      mq.addListener(onSystem);
    }
  }
  document.addEventListener("homebase:prefs-changed", function () {
    term.options.theme = homebaseTermTheme();
    term.options.cursorBlink = homebasePref("cursorBlink", true);
    term.options.scrollback = homebasePref("scrollback", 4000);
    const size = homebasePref("fontSize", 13);
    if (term.options.fontSize !== size) {
      term.options.fontSize = size;
      // Cell metrics changed, so the grid must be remeasured and the new
      // dimensions pushed to the PTY.
      document.dispatchEvent(new CustomEvent("homebase:refit"));
    }
  });
  return { term: term, fit: fit };
}

// Copy selected text to the *browser device* clipboard. Never sent to the
// PTY or the remote host. execCommand first because the default listen
// address is HTTP on Tailscale IPv4 (not a secure context).
function homebaseCopyText(text) {
  if (!text) {
    return false;
  }
  let ok = false;
  const onCopy = function (e) {
    if (!e.clipboardData) {
      return;
    }
    e.clipboardData.setData("text/plain", text);
    e.preventDefault();
    ok = true;
  };
  document.addEventListener("copy", onCopy);
  try {
    ok = document.execCommand("copy") || ok;
  } catch (e) {
    /* Clipboard API below */
  } finally {
    document.removeEventListener("copy", onCopy);
  }
  if (ok) {
    return true;
  }
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).catch(function () {});
    return true;
  }
  return false;
}

function homebaseBindCopyOnSelect(term) {
  let dragging = false;

  function copySelection() {
    // Read the preference at copy time, not at bind time, so toggling it in
    // Settings takes effect on the open terminal without a reload.
    if (!homebasePref("copyOnSelect", true)) {
      return;
    }
    if (!term.hasSelection()) {
      return;
    }
    const text = term.getSelection();
    if (text && homebaseCopyText(text)) {
      document.dispatchEvent(new CustomEvent("homebase:copied"));
    }
  }

  const el = term.element;
  el.addEventListener("mousedown", function (ev) {
    if (ev.button === 0) {
      dragging = true;
    }
  });
  el.addEventListener("touchstart", function () {
    dragging = true;
  }, { passive: true });
  document.addEventListener("mouseup", function (ev) {
    if (!dragging || ev.button !== 0) {
      return;
    }
    dragging = false;
    copySelection();
  });
  document.addEventListener("touchend", function () {
    if (!dragging) {
      return;
    }
    dragging = false;
    copySelection();
  });
  el.addEventListener("keyup", function (ev) {
    if (ev.shiftKey || ev.key === "Shift") {
      copySelection();
    }
  });
}

// xterm measures its scroll bar once, at construction, as
//   viewportElement.offsetWidth - scrollArea.offsetWidth || 15
// and FitAddon subtracts that from the usable width on every fit. Every
// touch platform draws overlay scroll bars, so that difference is 0, the
// `|| 15` fallback wins, and 15px -- about two columns on a phone -- stay
// dead for the life of the terminal. Re-derive the grid from the cell size
// the fit just proved, and give those columns back. Where a scroll bar is
// real (a desktop mouse) it occupies width, clientWidth already excludes it,
// and this comes out the same as FitAddon's answer.
function homebaseReclaimGutter(term) {
  const wrap = term.element && term.element.parentElement;
  const screen = term.element && term.element.querySelector(".xterm-screen");
  if (!wrap || !screen || !term.cols || !term.rows) {
    return;
  }
  const cellW = screen.offsetWidth / term.cols;
  const cellH = screen.offsetHeight / term.rows;
  if (!(cellW > 0) || !(cellH > 0)) {
    return;
  }
  const cols = Math.max(2, Math.floor(wrap.clientWidth / cellW));
  const rows = Math.max(1, Math.floor(wrap.clientHeight / cellH));
  if (cols !== term.cols || rows !== term.rows) {
    term.resize(cols, rows);
  }
}

// Rendered height of one row, for turning a finger's travel into a line
// count. Derived from what the renderer actually drew, not from the font
// size, because line-height and device pixel rounding both move it.
function homebaseCellHeight(term) {
  const screen = term.element && term.element.querySelector(".xterm-screen");
  if (!screen || !term.rows) {
    return 0;
  }
  const h = screen.offsetHeight / term.rows;
  return h > 0 ? h : 0;
}

function homebaseFitSize(fit, term) {
  try {
    fit.fit();
    homebaseReclaimGutter(term);
  } catch (e) {
    return { cols: term.cols || 80, rows: term.rows || 24 };
  }
  return {
    cols: term.cols || 80,
    rows: term.rows || 24
  };
}

// Whether the program in the foreground has asked for mouse reports. A TUI
// that has -- Claude Code (DECSET 1000 + 1006), vim, htop, less -- draws its
// own scrollback and expects the wheel; tmux's copy-mode history is not what
// a scroll should move there, and on the alternate screen there is barely any
// history to move anyway. xterm parsed those DECSETs out of the PTY stream,
// so `modes` answers for whatever is in the foreground right now. With tmux's
// own `mouse` off (what Homebase runs with) tmux forwards the pane's mouse
// mode to the client, so this tracks the program, not tmux.
function homebaseMouseReporting(term) {
  const modes = term.modes;
  return !!(modes && modes.mouseTrackingMode && modes.mouseTrackingMode !== "none");
}

// Turn `lines` of travel (positive = back into history) into wheel reports for
// the program, and let xterm encode them: which protocol and which extension
// were negotiated is its business, not ours.
//
// Two details are load-bearing. One report per line, because
// triggerMouseEvent sends exactly one report per event no matter how large
// the delta -- a drag is a run of notches, the way a trackpad produces them.
// And deltaMode LINE, because the pixel path divides by the row height and
// carries a remainder, which would drop notches.
function homebaseSendWheel(term, lines, x, y) {
  const target = term.element && term.element.querySelector(".xterm-screen");
  if (!target || !lines) {
    return false;
  }
  const step = lines > 0 ? -1 : 1; // wheel up (negative deltaY) reveals history
  const n = Math.min(Math.abs(lines), 40); // a flick must not become a burst
  for (let i = 0; i < n; i++) {
    target.dispatchEvent(new WheelEvent("wheel", {
      deltaY: step,
      deltaMode: 1, // WheelEvent.DOM_DELTA_LINE
      clientX: x,
      clientY: y,
      bubbles: true,
      cancelable: true
    }));
  }
  return true;
}

// Recover from an IME that leaves xterm composing forever.
//
// xterm tracks composition from DOM events alone: `compositionstart` sets a
// flag, `compositionend` clears it. While the flag is set, keydown with
// keyCode 229 is swallowed -- and on a phone keyboard *every* key reports 229.
// So one missing `compositionend`, which is what switching the iOS keyboard
// from Chinese to English produces, and nothing can be typed again for the
// life of the page. Nothing in xterm's public API can clear that flag.
//
// The rule this guard applies needs no knowledge of which key or which IME:
// text reached the textarea, no composition is in progress, and nothing
// reached the PTY -- then xterm is stuck. Clear the textarea, hand xterm the
// `compositionend` it never got (it reads the now-empty textarea and sends
// nothing, so this cannot double up), and send what was typed ourselves.
const HOMEBASE_IME_STUCK_MS = 80; // xterm's own send paths are sync or setTimeout(0)
// Generous on purpose. Too short and a slow typist on an IME that misreports
// isComposing gets their raw pinyin sent; too long only costs a stuck user one
// more keystroke, since every keystroke re-arms the check.
const HOMEBASE_IME_LIVE_MS = 1500;
const HOMEBASE_IME_MAX_REPLAY = 16; // a keystroke or a committed word, not a backlog

function homebaseGuardIME(term, send) {
  const ta = term.textarea;
  if (!ta) {
    return;
  }
  // xterm only clears the textarea on blur, Enter and ^C, so its value is a
  // running log. `accounted` is how much of it has already reached the PTY.
  let accounted = "";
  let lastComposition = 0;
  let dataCount = 0;
  let selfDispatch = false;

  term.onData(function () {
    dataCount++;
    accounted = ta.value;
  });
  // Deliberately not a boolean: a flag set by compositionstart and cleared by
  // compositionend would go stale on exactly the missing event this guard
  // exists to survive. A timestamp cannot -- a composition in progress emits
  // an event per keystroke, so silence for long enough means there is none,
  // whatever xterm still believes.
  ["compositionstart", "compositionupdate", "compositionend"].forEach(function (name) {
    ta.addEventListener(name, function () {
      if (!selfDispatch) { // the guard's own compositionend is not evidence of one
        lastComposition = Date.now();
      }
    });
  });

  // xterm's own paste handler sends the clipboard and calls stopPropagation --
  // but never preventDefault, so the browser goes on to insert the same text
  // into the textarea, which fires `input` with nothing following it on the
  // data channel. That looks exactly like a swallowed keystroke, so the guard
  // replayed it and Cmd+V arrived twice. Not every time: only pastes short
  // enough to clear HOMEBASE_IME_MAX_REPLAY, and only when nothing else was
  // typed within HOMEBASE_IME_STUCK_MS. Cancelling the insert keeps the
  // textarea a faithful log of what was *typed*; paste is already accounted
  // for by the time this runs.
  ta.addEventListener("paste", function (ev) {
    ev.preventDefault();
  });

  ta.addEventListener("input", function (ev) {
    // Mid-composition silence is correct: the pinyin is not input yet.
    if (ev.isComposing) {
      return;
    }
    // Belt and braces for a browser that inserted the paste anyway: xterm has
    // already sent it, so only the bookkeeping needs to catch up.
    if (ev.inputType === "insertFromPaste" || ev.inputType === "insertFromDrop") {
      accounted = ta.value;
      return;
    }
    const mark = dataCount;
    setTimeout(function () {
      if (dataCount !== mark) {
        return; // xterm handled it after all
      }
      if (Date.now() - lastComposition < HOMEBASE_IME_LIVE_MS) {
        return; // a live composition, including one whose events trail the input
      }
      const value = ta.value;
      let pending = "";
      if (value.length > accounted.length && value.indexOf(accounted) === 0) {
        pending = value.slice(accounted.length);
      } else if (accounted.indexOf(value) === 0 && accounted.length === value.length + 1) {
        pending = "\x7f"; // a swallowed Backspace
      }
      // Enter reaches the textarea as a line break, but a PTY wants CR.
      pending = pending.replace(/\n/g, "\r");
      ta.value = "";
      accounted = "";
      selfDispatch = true;
      ta.dispatchEvent(new Event("compositionend"));
      selfDispatch = false;
      if (pending && pending.length <= HOMEBASE_IME_MAX_REPLAY) {
        send(pending);
      }
    }, HOMEBASE_IME_STUCK_MS);
  });
}
