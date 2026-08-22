// Per-device UI preferences: theme, copy-on-select, terminal look.
//
// These are deliberately NOT server config. They live in this browser's
// localStorage, so every device gets its own answer (a phone in a dark room
// and a desktop in daylight want different themes) and so changing them needs
// no loopback privilege — unlike the server settings API, which decides the
// listen address and stays 127.0.0.1-only.
//
// Loaded in <head> so the theme is on <html> before first paint, no flash.
(function () {
  const KEY = "homebase.prefs";

  const DEFAULTS = {
    theme: "system",      // "system" | "light" | "dark"
    copyOnSelect: true,   // selecting text copies it to this device's clipboard
    fontSize: 13,         // terminal font size in px
    scrollback: 4000,     // terminal scrollback lines
    cursorBlink: true,
    doubleTapTab: true,   // double-tapping the terminal on a touch screen sends Tab
    newWindowDir: "same"  // "same" as the current window's cwd, or "home"
  };

  const THEMES = ["system", "light", "dark"];
  const THEME_CYCLE = ["system", "dark", "light"];
  const SCROLLBACKS = [1000, 4000, 10000, 50000];
  const NEW_WINDOW_DIRS = ["same", "home"];
  const FONT_MIN = 10;
  const FONT_MAX = 22;

  function clean(raw) {
    const out = {
      theme: DEFAULTS.theme,
      copyOnSelect: DEFAULTS.copyOnSelect,
      fontSize: DEFAULTS.fontSize,
      scrollback: DEFAULTS.scrollback,
      cursorBlink: DEFAULTS.cursorBlink,
      doubleTapTab: DEFAULTS.doubleTapTab,
      newWindowDir: DEFAULTS.newWindowDir
    };
    if (!raw || typeof raw !== "object") {
      return out;
    }
    if (THEMES.indexOf(raw.theme) >= 0) {
      out.theme = raw.theme;
    }
    if (typeof raw.copyOnSelect === "boolean") {
      out.copyOnSelect = raw.copyOnSelect;
    }
    if (typeof raw.cursorBlink === "boolean") {
      out.cursorBlink = raw.cursorBlink;
    }
    if (typeof raw.doubleTapTab === "boolean") {
      out.doubleTapTab = raw.doubleTapTab;
    }
    const n = Number(raw.fontSize);
    if (isFinite(n) && n >= FONT_MIN && n <= FONT_MAX) {
      out.fontSize = Math.round(n);
    }
    if (SCROLLBACKS.indexOf(Number(raw.scrollback)) >= 0) {
      out.scrollback = Number(raw.scrollback);
    }
    if (NEW_WINDOW_DIRS.indexOf(raw.newWindowDir) >= 0) {
      out.newWindowDir = raw.newWindowDir;
    }
    return out;
  }

  function read() {
    try {
      return clean(JSON.parse(localStorage.getItem(KEY)));
    } catch (e) {
      return clean(null); // private mode, or corrupted value
    }
  }

  let state = read();

  function write() {
    try {
      localStorage.setItem(KEY, JSON.stringify(state));
    } catch (e) { /* private mode: preferences last for this page only */ }
  }

  // "system" leaves the attribute off so the CSS media query decides.
  function applyTheme() {
    const root = document.documentElement;
    if (state.theme === "system") {
      root.removeAttribute("data-theme");
    } else {
      root.setAttribute("data-theme", state.theme);
    }
  }

  function emit() {
    document.dispatchEvent(new CustomEvent("homebase:prefs-changed", {
      detail: { prefs: all() }
    }));
  }

  function all() {
    return {
      theme: state.theme,
      copyOnSelect: state.copyOnSelect,
      fontSize: state.fontSize,
      scrollback: state.scrollback,
      cursorBlink: state.cursorBlink,
      doubleTapTab: state.doubleTapTab,
      newWindowDir: state.newWindowDir
    };
  }

  function get(key) {
    return state[key];
  }

  function set(key, value) {
    const next = all();
    next[key] = value;
    const cleaned = clean(next);
    if (cleaned[key] === state[key]) {
      return state[key];
    }
    state = cleaned;
    write();
    applyTheme();
    emit();
    return state[key];
  }

  // system → dark → light → system. The control lives next to Settings on
  // the terminal page, not in the Settings form.
  function cycleTheme() {
    const i = THEME_CYCLE.indexOf(state.theme);
    return set("theme", THEME_CYCLE[(i + 1) % THEME_CYCLE.length]);
  }

  // Another tab changed preferences: follow along without a reload.
  window.addEventListener("storage", function (ev) {
    if (ev.key !== KEY) {
      return;
    }
    state = read();
    applyTheme();
    emit();
  });

  window.homebasePrefs = {
    DEFAULTS: DEFAULTS,
    THEMES: THEMES,
    SCROLLBACKS: SCROLLBACKS,
    NEW_WINDOW_DIRS: NEW_WINDOW_DIRS,
    FONT_MIN: FONT_MIN,
    FONT_MAX: FONT_MAX,
    all: all,
    get: get,
    set: set,
    cycleTheme: cycleTheme
  };

  applyTheme();
})();
