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

function homebaseFitSize(fit, term) {
  try {
    fit.fit();
  } catch (e) {
    return { cols: term.cols || 80, rows: term.rows || 24 };
  }
  return {
    cols: term.cols || 80,
    rows: term.rows || 24
  };
}
