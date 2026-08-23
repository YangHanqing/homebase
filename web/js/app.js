(function () {
  const appEl = document.getElementById("app");
  const listEl = document.getElementById("window-list");
  const listMsg = document.getElementById("list-msg");
  const termWrap = document.getElementById("term-wrap");
  const paneStatus = document.getElementById("pane-status");
  const paneStatusLine = document.getElementById("pane-status-line");
  const paneStatusHelp = document.getElementById("pane-status-help");
  const navToggle = document.getElementById("nav-toggle");
  const scrim = document.getElementById("scrim");
  const chromeHost = document.getElementById("chrome-host");
  const chromeLed = document.getElementById("chrome-led");
  const sideLed = document.getElementById("side-led");
  const sideState = document.getElementById("side-state");
  const sideClients = document.getElementById("side-clients");
  const copyToast = document.getElementById("copy-toast");

  const renameModal = document.getElementById("rename-modal");
  const renameForm = document.getElementById("rename-form");
  const renameFormErr = document.getElementById("rename-form-err");

  const POLL_MS = 3000;
  const t = window.homebaseI18n.t;

  // One terminal, one WebSocket, for the whole page.
  const created = homebaseCreateTerminal(termWrap);
  const term = created.term;
  const fit = created.fit;

  let windows = [];
  let session = null;
  let pollTimer = null;
  let inFlight = false;
  let lastStatus = { state: "connecting" };

  const mobileMq = window.matchMedia("(max-width: 720px), (max-height: 500px)");

  const HELP = {
    enotmux: "help.enotmux",
    pty_spawn: "help.pty_spawn",
    ws_closed: "help.ws_closed"
  };

  const LABEL = {
    connected: "status.connected",
    connecting: "status.connecting",
    disconnected: "status.disconnected",
    error: "status.error"
  };

  function isMobile() {
    return mobileMq.matches;
  }

  // Only the REST handlers answer in JSON. The auth gate serves HTML, and
  // net/http's own rejections (bad host, method not allowed) are plain text,
  // so parsing every response as JSON turned a revoked device into
  // "Unexpected token '<'" in the sidebar — precisely the moment the user
  // most needs to be told what happened.
  let reloading = false;

  function unpaired() {
    if (!reloading) {
      reloading = true;
      // The gate answers "/" with the pairing instructions; a reload is what
      // puts them on screen.
      location.reload();
    }
    const err = new Error(t("list.unpaired"));
    err.status = 401;
    return err;
  }

  function api(path, opts) {
    return fetch(path, opts).then(function (res) {
      if (res.status === 401) {
        throw unpaired();
      }
      if (res.status === 204) {
        return null;
      }
      return res.text().then(function (text) {
        let body = null;
        try {
          body = text ? JSON.parse(text) : null;
        } catch (e) {
          body = null;
        }
        if (!res.ok) {
          const plain = body ? "" : text.trim().split("\n")[0].slice(0, 200);
          const err = new Error(
            (body && body.error) || plain || t("list.serverError") || res.statusText
          );
          err.status = res.status;
          err.code = body && body.code;
          throw err;
        }
        return body;
      });
    });
  }

  function el(tag, cls) {
    const n = document.createElement(tag);
    if (cls) {
      n.className = cls;
    }
    return n;
  }

  function setNavOpen(open) {
    appEl.classList.toggle("is-nav-open", open);
    scrim.hidden = !open;
    navToggle.setAttribute("aria-expanded", open ? "true" : "false");
    navToggle.setAttribute("aria-label", open ? t("aria.closeWindows") : t("aria.openWindows"));
    if (!open) {
      term.focus();
    }
  }

  // ---- status -------------------------------------------------------------

  function renderStatus(st) {
    lastStatus = st || lastStatus;
    const cls = "led " + (st.state || "");
    sideLed.className = cls;
    chromeLed.className = cls;
    // The led dot already carries "connected" (green); the word next to it
    // is only useful for the states where color alone isn't enough context.
    sideState.textContent = st.state === "connected" ? "" : (t(LABEL[st.state]) || st.state || "");
    sideState.title = st.state === "disconnected" ? t("status.disconnectedHint") : "";

    const bad = st.state === "error" ||
      (st.state === "disconnected" && st.code && st.code !== "ws_closed");
    if (st.state === "connected") {
      paneStatus.hidden = true;
      return;
    }
    paneStatus.hidden = false;
    paneStatusLine.textContent = bad && st.message
      ? st.message
      : (t(LABEL[st.state]) || t("status.connecting"));
    paneStatusHelp.textContent = bad ? (t(HELP[st.code]) || "") : "";
  }

  function syncChrome() {
    const active = activeWindow();
    chromeHost.textContent = active ? active.name : "";
  }

  function activeWindow() {
    for (let i = 0; i < windows.length; i++) {
      if (windows[i].active) {
        return windows[i];
      }
    }
    return null;
  }

  // ---- the single session -------------------------------------------------

  function connect() {
    if (session) {
      session.dispose();
      session = null;
    }
    term.reset();
    session = new HomebaseSession({
      term: term,
      onStatus: function (st) {
        renderStatus(st);
        if (st.state === "connected") {
          // The session may have just been created by this very connect.
          refreshWindows();
        }
      }
    });
    const size = homebaseFitSize(fit, term);
    session.setSize(size.cols, size.rows);
    session.connect();
  }

  // ---- tmux windows -------------------------------------------------------

  function renderWindows() {
    listEl.innerHTML = "";
    const onlyOne = windows.length <= 1;
    windows.forEach(function (w) {
      const li = el("li", "plate" + (w.active ? " is-active" : ""));

      const main = el("button", "plate-main");
      main.type = "button";
      const nameEl = el("div", "plate-name");
      nameEl.textContent = w.name;
      const idxEl = el("div", "plate-index");
      idxEl.textContent = w.index;
      main.appendChild(idxEl);
      main.appendChild(nameEl);
      main.addEventListener("click", function () {
        selectWindow(w.index);
      });

      const actions = el("div", "plate-actions");
      const renameBtn = el("button", "btn-tiny");
      renameBtn.type = "button";
      renameBtn.textContent = "✎";
      renameBtn.title = t("window.rename");
      renameBtn.setAttribute("aria-label", t("window.renameAria", { name: w.name }));
      renameBtn.addEventListener("click", function () {
        openRename(w);
      });
      const killBtn = el("button", "btn-tiny");
      killBtn.type = "button";
      killBtn.textContent = "×";
      killBtn.title = onlyOne ? t("window.closeLastTitle") : t("window.closeTitle");
      killBtn.setAttribute("aria-label", t("window.closeAria", { name: w.name }));
      killBtn.disabled = onlyOne;
      killBtn.addEventListener("click", function () {
        killWindow(w.index);
      });
      actions.appendChild(renameBtn);
      actions.appendChild(killBtn);

      li.appendChild(main);
      li.appendChild(actions);
      listEl.appendChild(li);
    });
    syncChrome();
  }

  function showListMsg(text) {
    listMsg.textContent = text || "";
    listMsg.hidden = !text;
  }

  // Anyone attached to the tmux session — another Homebase tab, or a plain
  // `tmux attach` over ssh — can shrink this view via tmux's smallest-client
  // resize behavior, so surface the raw count rather than just "connected".
  function renderClients(count) {
    if (!count) {
      sideClients.hidden = true;
      return;
    }
    sideClients.hidden = false;
    sideClients.textContent = t("sidebar.clients", { count: count });
    sideClients.title = t("sidebar.clientsHint", { count: count });
    sideClients.classList.toggle("is-crowded", count > 1);
  }

  function windowsPath(suffix) {
    return "/api/windows" + (suffix || "");
  }

  function refreshWindows() {
    if (inFlight) {
      return Promise.resolve();
    }
    inFlight = true;
    return api(windowsPath()).then(function (body) {
      windows = (body && body.windows) || [];
      showListMsg(windows.length ? "" : t("list.empty"));
      renderWindows();
      renderClients(body && body.clients);
    }).catch(function (err) {
      showListMsg(err.message);
    }).finally(function () {
      inFlight = false;
    });
  }

  function act(promise) {
    return promise.then(function () {
      return refreshWindows();
    }).catch(function (err) {
      showListMsg(err.message);
    });
  }

  function selectWindow(index) {
    // Optimistic: tmux redraws through the PTY faster than we can re-list.
    windows.forEach(function (w) { w.active = w.index === index; });
    renderWindows();
    if (isMobile()) {
      setNavOpen(false);
    }
    act(api(windowsPath("/" + index), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ active: true })
    }));
  }

  function newWindow() {
    act(api(windowsPath(), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ dir: homebasePref("newWindowDir", "same") })
    }));
  }

  function killWindow(index) {
    act(api(windowsPath("/" + index), { method: "DELETE" }));
  }

  // ---- rename modal ---------------------------------------------------------

  function openRename(w) {
    renameForm.reset();
    renameFormErr.hidden = true;
    document.getElementById("r-index").value = w.index;
    document.getElementById("r-name").value = w.name;
    renameModal.hidden = false;
    document.getElementById("r-name").focus();
    document.getElementById("r-name").select();
  }

  renameForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    renameFormErr.hidden = true;
    const index = document.getElementById("r-index").value;
    const name = document.getElementById("r-name").value;
    api(windowsPath("/" + encodeURIComponent(index)), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: name })
    }).then(function () {
      renameModal.hidden = true;
      refreshWindows();
    }).catch(function (err) {
      renameFormErr.textContent = err.message;
      renameFormErr.hidden = false;
    });
  });

  // ---- wiring -------------------------------------------------------------

  let toastTimer = null;
  document.addEventListener("homebase:copied", function () {
    copyToast.hidden = false;
    copyToast.textContent = t("toast.copied");
    // Restart the fade-in on repeated copies instead of stacking timers.
    copyToast.classList.remove("is-shown");
    void copyToast.offsetWidth;
    copyToast.classList.add("is-shown");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      copyToast.classList.remove("is-shown");
      copyToast.hidden = true;
    }, 1400);
  });

  document.getElementById("btn-window-add").addEventListener("click", newWindow);
  document.getElementById("btn-rename-cancel").addEventListener("click", function () {
    renameModal.hidden = true;
  });
  renameModal.addEventListener("click", function (ev) {
    if (ev.target === renameModal) {
      renameModal.hidden = true;
    }
  });
  navToggle.addEventListener("click", function () {
    setNavOpen(!appEl.classList.contains("is-nav-open"));
  });
  scrim.addEventListener("click", function () {
    setNavOpen(false);
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key !== "Escape") {
      return;
    }
    if (!renameModal.hidden) {
      renameModal.hidden = true;
      return;
    }
    if (!keysModal.hidden) {
      setKeysOpen(false);
      return;
    }
    if (appEl.classList.contains("is-nav-open")) {
      setNavOpen(false);
    }
  });
  document.addEventListener("homebase:lang-changed", function () {
    renderWindows();
    renderStatus(lastStatus);
    renderThemeButtons();
  });

  function renderThemeButtons() {
    const theme = window.homebasePrefs.get("theme");
    const name = t("theme." + theme);
    const title = t("theme.title", { name: name });
    document.querySelectorAll("[data-theme-cycle]").forEach(function (btn) {
      btn.textContent = name;
      btn.title = title;
      btn.setAttribute("aria-label", title);
    });
  }

  document.querySelectorAll("[data-theme-cycle]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      window.homebasePrefs.cycleTheme();
    });
  });
  document.addEventListener("homebase:prefs-changed", renderThemeButtons);
  renderThemeButtons();

  // ---- sizing -------------------------------------------------------------

  let resizeTimer = null;
  const ro = new ResizeObserver(function () {
    if (resizeTimer) {
      clearTimeout(resizeTimer);
    }
    resizeTimer = setTimeout(function () {
      const size = homebaseFitSize(fit, term);
      if (session) {
        session.setSize(size.cols, size.rows);
      }
    }, 32);
  });
  ro.observe(document.getElementById("pane"));

  // Changing the terminal font size resizes the grid without resizing the
  // pane, so the ResizeObserver above never fires for it.
  document.addEventListener("homebase:refit", function () {
    const size = homebaseFitSize(fit, term);
    if (session) {
      session.setSize(size.cols, size.rows);
    }
  });

  function syncAppHeight() {
    const h = (window.visualViewport && window.visualViewport.height) || window.innerHeight;
    appEl.style.setProperty("--app-height", h + "px");
  }
  syncAppHeight();
  window.addEventListener("resize", syncAppHeight);
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", syncAppHeight);
  }

  // ---- polling ------------------------------------------------------------

  function startPolling() {
    stopPolling();
    pollTimer = setInterval(function () {
      if (document.visibilityState === "hidden") {
        return;
      }
      refreshWindows();
    }, POLL_MS);
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState !== "visible") {
      return;
    }
    // A phone that slept for an hour comes back with a dead socket and a
    // stale list. Wake both rather than waiting for the backoff timer.
    if (session) {
      session.wake();
    }
    refreshWindows();
  });

  // ---- terminal input -----------------------------------------------------

  // On-screen keyboards can't produce Ctrl+<key> or Alt+<key>. "Ctrl" and
  // "Alt" arm a one-shot modifier: the next character typed (from any
  // keyboard, any input method) passes through term.onData as plain text
  // regardless of how it was produced, so turning it into a control byte or
  // an ESC-prefixed sequence works even where real keydown metadata
  // (ctrlKey, key code) is unreliable on mobile.
  const armed = { ctrl: false, alt: false };

  function setArmed(mod, v) {
    armed[mod] = v;
    document.querySelectorAll('[data-mod="' + mod + '"]').forEach(function (btn) {
      btn.classList.toggle("is-active", v);
    });
  }

  function disarm() {
    setArmed("ctrl", false);
    setArmed("alt", false);
  }

  function ctrlByte(ch) {
    const c = ch.toUpperCase().charCodeAt(0);
    if (c >= 64 && c <= 95) {
      return String.fromCharCode(c - 64);
    }
    if (ch === "?") {
      return String.fromCharCode(127);
    }
    return null;
  }

  // Keys typed into copy mode are eaten as copy-mode commands, so a scroll has
  // to be cancelled before the first keystroke lands. A tap does that on the
  // phone; a wheel user just starts typing. The cancel travels on the control
  // channel and the input on the WebSocket with nothing ordering the two, so
  // the input waits for the cancel to return -- and whatever is typed behind
  // it queues on the same promise, or a fast burst would arrive out of order.
  let pendingInput = null;

  function afterCopyMode(fn) {
    if (inCopyMode) {
      pendingInput = leaveCopyMode();
    }
    if (!pendingInput) {
      fn();
      return;
    }
    const chain = pendingInput.then(fn);
    pendingInput = chain;
    chain.then(function () {
      if (pendingInput === chain) {
        pendingInput = null; // drained: back to sending straight through
      }
    });
  }

  // A tap has nothing to send, but it still cancels copy mode -- and the queue
  // has to know, or the Tab a double-tap sends would overtake the cancel.
  function queueLeaveCopyMode() {
    afterCopyMode(function () {});
  }

  function sendInput(data) {
    if (!session) {
      return;
    }
    afterCopyMode(function () {
      if (session) {
        session.sendInput(data);
      }
    });
  }

  function typed(data) {
    if (!session) {
      return;
    }
    // Mouse reports come through onData too (that is how a wheel notch
    // reaches the program). They are not something the user typed, so a
    // sticky modifier must neither be applied to them nor spent on them.
    if ((!armed.ctrl && !armed.alt) || data.slice(0, 3) === "\x1b[<" || data.slice(0, 3) === "\x1b[M") {
      sendInput(data);
      return;
    }
    let out = data;
    if (armed.ctrl && data.length === 1) {
      const byte = ctrlByte(data);
      if (byte !== null) {
        out = byte;
      }
    }
    if (armed.alt) {
      out = "\x1b" + out; // Meta as ESC prefix, what readline and TUIs expect
    }
    disarm();
    sendInput(out);
  }

  term.onData(typed);

  // A phone keyboard can leave xterm composing forever; the guard notices and
  // replays what was swallowed. Through `typed`, so a replayed keystroke gets
  // the same sticky modifier and copy-mode treatment as a live one.
  homebaseGuardIME(term, typed);

  // Sequences for keys an on-screen keyboard has no way to send. Home/End
  // use the normal-mode forms; the rest are the standard xterm codes.
  const KEY_SEQ = {
    Escape: "\x1b",
    Tab: "\t",
    "S-Tab": "\x1b[Z",
    ArrowUp: "\x1b[A",
    ArrowDown: "\x1b[B",
    ArrowRight: "\x1b[C",
    ArrowLeft: "\x1b[D",
    Home: "\x1b[H",
    End: "\x1b[F",
    PageUp: "\x1b[5~",
    PageDown: "\x1b[6~",
    F1: "\x1bOP", F2: "\x1bOQ", F3: "\x1bOR", F4: "\x1bOS",
    F5: "\x1b[15~", F6: "\x1b[17~", F7: "\x1b[18~", F8: "\x1b[19~",
    F9: "\x1b[20~", F10: "\x1b[21~", F11: "\x1b[23~", F12: "\x1b[24~"
  };

  // "C-x" spells the control byte directly, for the keys that earn a button
  // of their own (^C on the toolbar, the whole readline row in the panel)
  // rather than costing two taps through the sticky Ctrl.
  function keySeq(name) {
    if (KEY_SEQ[name]) {
      return KEY_SEQ[name];
    }
    if (name.length === 3 && name.slice(0, 2) === "C-") {
      return ctrlByte(name[2]);
    }
    return null;
  }

  // A key or symbol button sends exactly what it says. Any armed modifier is
  // dropped rather than silently applied: "Ctrl" then "F5" has no agreed
  // meaning, and a stale armed modifier would corrupt the next thing typed.
  function bindKeyButtons(root) {
    root.querySelectorAll("[data-key], [data-text]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        const text = btn.getAttribute("data-text");
        const seq = text !== null ? text : keySeq(btn.getAttribute("data-key"));
        if (seq && session) {
          disarm();
          sendInput(seq);
        }
        term.focus();
      });
    });
    root.querySelectorAll("[data-mod]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        const mod = btn.getAttribute("data-mod");
        setArmed(mod, !armed[mod]);
        term.focus();
      });
    });
  }
  bindKeyButtons(document);

  // ---- the "all keys" panel -----------------------------------------------

  const keysModal = document.getElementById("keys-modal");
  const keyMore = document.getElementById("key-more");

  function setKeysOpen(open) {
    keysModal.hidden = !open;
    keyMore.classList.toggle("is-active", open);
    if (!open) {
      term.focus();
    }
  }

  keyMore.addEventListener("click", function () {
    setKeysOpen(keysModal.hidden);
  });
  document.getElementById("btn-keys-close").addEventListener("click", function () {
    setKeysOpen(false);
  });
  // Tapping outside the sheet closes it; taps on keys inside must not, so
  // that a run of ^R presses needs one trip through the panel, not five.
  keysModal.addEventListener("click", function (ev) {
    if (ev.target === keysModal) {
      setKeysOpen(false);
    }
  });

  // ---- touch gestures on the terminal -------------------------------------

  // One handler owns every finger on the terminal, and it has to run in the
  // capture phase on the wrapper, ahead of xterm's own listeners on .xterm.
  //
  // The reason is a single upstream behaviour that breaks both scrolling and
  // selection. xterm's touchmove listener is:
  //
  //     viewport.handleTouchMove(e) ? undefined : this.cancel(e)
  //
  // where cancel() is preventDefault + stopPropagation. handleTouchMove
  // returns false when the viewport has nothing to scroll -- and it never has
  // anything to scroll here, because we are attached to a full-screen tmux
  // client, which lives on the alternate screen and gives xterm no scrollback
  // of its own. So on this page xterm cancels *every* touchmove over the
  // terminal, which kills native scrolling and kills iOS's long-press
  // selection drag with it. Capturing first and stopping propagation is what
  // keeps the gesture ours to decide.
  const TAP_MS = 300;         // double-tap window
  const TAP_SLOP = 24;        // px a double-tap may wander
  const DRAG_SLOP = 8;        // px before a move counts as a scroll
  const LONG_PRESS_MS = 450;  // iOS raises its selection magnifier at ~500ms

  let gesture = null;
  let lastTap = 0;
  let lastTapX = 0;
  let lastTapY = 0;

  termWrap.addEventListener("touchstart", function (ev) {
    if (ev.touches.length !== 1) {
      gesture = null; // pinch-to-zoom and friends are the browser's business
      return;
    }
    const touch = ev.touches[0];
    gesture = {
      x: touch.clientX,
      y: touch.clientY,
      lastY: touch.clientY,
      at: Date.now(),
      mode: null
    };
  }, { capture: true, passive: true });

  termWrap.addEventListener("touchmove", function (ev) {
    if (!gesture || ev.touches.length !== 1) {
      return;
    }
    const touch = ev.touches[0];
    if (gesture.mode === null) {
      // A finger that has been still this long is placing a selection, not
      // starting a scroll. Anything else that has travelled mostly vertically
      // is a scroll.
      const dx = Math.abs(touch.clientX - gesture.x);
      const dy = Math.abs(touch.clientY - gesture.y);
      if (Date.now() - gesture.at > LONG_PRESS_MS) {
        gesture.mode = "select";
      } else if (dy > DRAG_SLOP && dy > dx) {
        gesture.mode = "scroll";
      } else {
        return;
      }
    }
    if (gesture.mode === "select") {
      // Hands the gesture to iOS: no preventDefault, so the magnifier and the
      // Copy callout behave as they do in any other app. Propagation still
      // has to stop, or xterm cancels the move out from under the selection.
      ev.stopPropagation();
      return;
    }
    ev.stopPropagation();
    ev.preventDefault();
    const cell = homebaseCellHeight(term);
    if (!cell) {
      return;
    }
    // Dragging down reveals what is above, like every scrolling list.
    const lines = Math.trunc((touch.clientY - gesture.lastY) / cell);
    if (lines) {
      gesture.lastY += lines * cell;
      scrollBy(lines, touch.clientX, touch.clientY);
    }
  }, { capture: true, passive: false });

  termWrap.addEventListener("touchend", function (ev) {
    const g = gesture;
    gesture = null;
    if (!g || g.mode !== null || ev.changedTouches.length !== 1) {
      return; // a scroll or a selection, not a tap
    }
    // A tap is what you do before typing, so it is also the moment to come
    // back from wherever the scrollback wandered. Through the queue, so the
    // double-tap Tab below cannot overtake the cancel it depends on.
    queueLeaveCopyMode();
    if (!window.homebasePrefs.get("doubleTapTab")) {
      return;
    }
    const touch = ev.changedTouches[0];
    const now = Date.now();
    if (now - lastTap < TAP_MS &&
        Math.abs(touch.clientX - lastTapX) < TAP_SLOP &&
        Math.abs(touch.clientY - lastTapY) < TAP_SLOP) {
      lastTap = 0;
      // Safari synthesises mouse events from a tap *after* touchend, and a
      // synthesised double-click would make xterm select a word -- which
      // copy-on-select would then put on the clipboard. preventDefault stops
      // the synthesis, so a tap that meant Tab leaves the clipboard alone.
      ev.preventDefault();
      if (session) {
        disarm();
        sendInput("\t");
      }
      return;
    }
    lastTap = now;
    lastTapX = touch.clientX;
    lastTapY = touch.clientY;
  }, { capture: true, passive: false });

  // ---- scrollback ---------------------------------------------------------

  // Where a scroll goes depends on who is in the foreground. A program that
  // asked for mouse reports (Claude Code, vim, htop, less) keeps its own
  // scrollback and expects wheel notches -- sending it to tmux copy-mode
  // instead moves the wrong thing, and on the alternate screen usually moves
  // nothing at all. Everything else means the shell, whose history is tmux's.
  function scrollBy(lines, x, y) {
    if (homebaseMouseReporting(term)) {
      homebaseSendWheel(term, lines, x, y);
      return;
    }
    queueScroll(lines);
  }

  // A mouse wheel has the same problem a swipe does: there is no browser
  // scrollback to move (see homebaseSendWheel). When the program wants mouse
  // reports the event is xterm's to encode, so leave it alone; otherwise take
  // it, because xterm's fallback for a buffer with no scrollback is to send
  // one arrow key per line -- which walks the shell's history instead of
  // scrolling. Sub-line deltas from a trackpad accumulate rather than round
  // away to nothing.
  let wheelCarry = 0;

  termWrap.addEventListener("wheel", function (ev) {
    if (homebaseMouseReporting(term)) {
      return;
    }
    ev.stopPropagation();
    ev.preventDefault();
    if (ev.shiftKey) {
      return; // xterm reads shift+wheel as "do not scroll"; so do we
    }
    let delta = ev.deltaY;
    if (ev.deltaMode === 2) {
      delta *= term.rows; // by the page
    } else if (ev.deltaMode !== 1) {
      delta /= homebaseCellHeight(term) || 1; // by the pixel
    }
    wheelCarry -= delta; // wheel down goes forward, which is negative lines
    const lines = Math.trunc(wheelCarry);
    if (lines) {
      wheelCarry -= lines;
      queueScroll(lines);
    }
  }, { capture: true, passive: false });

  // tmux owns the history, so a swipe is a REST call, not a local scroll. At
  // most one is in flight; whatever the finger covers meanwhile is summed and
  // sent as the next one, which keeps a fast flick from queueing a burst of
  // requests that would arrive long after the finger stopped.
  let pendingLines = 0;
  let scrollInFlight = false;
  let inCopyMode = false;

  function queueScroll(lines) {
    pendingLines += lines;
    flushScroll();
  }

  function flushScroll() {
    if (scrollInFlight || !pendingLines) {
      return;
    }
    const lines = pendingLines;
    pendingLines = 0;
    scrollInFlight = true;
    api("/api/scroll", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ lines: lines })
    }).then(function (res) {
      inCopyMode = !!(res && res.in_mode);
    }).catch(function () {
      // A failed scroll is not worth a banner; the view simply did not move.
    }).then(function () {
      scrollInFlight = false;
      flushScroll();
    });
  }

  // Swiping back to the bottom leaves copy mode on its own (tmux "copy-mode
  // -e"), so this is only for the other way out: the user tapped, and wants
  // to type at a live prompt rather than send keys into copy mode.
  // Resolves once tmux is back at the live output, so a caller that is about
  // to send input can wait: the cancel travels on the control channel and the
  // input on the WebSocket, and nothing orders those two against each other.
  function leaveCopyMode() {
    if (!inCopyMode) {
      return Promise.resolve();
    }
    inCopyMode = false;
    pendingLines = 0;
    return api("/api/scroll", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ lines: 0 })
    }).catch(function () {});
  }

  // ---- boot ---------------------------------------------------------------

  renderStatus({ state: "connecting", code: "", message: "" });
  connect();
  refreshWindows();
  startPolling();
  term.focus();
})();
