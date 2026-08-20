function homebaseCreateTerminal(container) {
  const term = new Terminal({
    cursorBlink: true,
    fontFamily: "ui-monospace, Menlo, monospace",
    fontSize: 13,
    lineHeight: 1.2,
    theme: {
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
    },
    scrollback: 4000,
    allowProposedApi: true
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  const unicode11 = new Unicode11Addon.Unicode11Addon();
  term.loadAddon(unicode11);
  term.unicode.activeVersion = "11";
  term.open(container);
  homebaseBindCopyOnSelect(term);
  return { term: term, fit: fit };
}

// Copy selected text to the *browser device* clipboard. Never sent to the
// PTY or the remote host. execCommand first because the default listen
// address is HTTP on Tailscale IPv4 (not a secure context).
function homebaseCopyText(text) {
  if (!text) {
    return;
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
    return;
  }
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).catch(function () {});
  }
}

function homebaseBindCopyOnSelect(term) {
  let dragging = false;

  function copySelection() {
    if (!term.hasSelection()) {
      return;
    }
    const text = term.getSelection();
    if (text) {
      homebaseCopyText(text);
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
