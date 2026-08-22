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

  function api(path, opts) {
    return fetch(path, opts).then(function (res) {
      if (res.status === 204) {
        return null;
      }
      return res.json().then(function (body) {
        if (!res.ok) {
          const err = new Error((body && body.error) || res.statusText);
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

  // On-screen keyboards can't produce Ctrl+<key>. "Ctrl" arms a one-shot
  // modifier: the next character typed (from any keyboard, any input method)
  // passes through here as plain text regardless of how it was produced, so
  // turning it into a control byte works even where real keydown metadata
  // (ctrlKey, key code) is unreliable on mobile.
  const keyCtrlBtn = document.getElementById("key-ctrl");
  let ctrlArmed = false;

  function setCtrlArmed(v) {
    ctrlArmed = v;
    if (keyCtrlBtn) {
      keyCtrlBtn.classList.toggle("is-active", v);
    }
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

  term.onData(function (data) {
    if (!session) {
      return;
    }
    if (ctrlArmed) {
      setCtrlArmed(false);
      const byte = data.length === 1 ? ctrlByte(data) : null;
      session.sendInput(byte === null ? data : byte);
      return;
    }
    session.sendInput(data);
  });

  if (keyCtrlBtn) {
    keyCtrlBtn.addEventListener("click", function () {
      setCtrlArmed(!ctrlArmed);
      term.focus();
    });
  }

  const KEY_SEQ = {
    Escape: "\x1b",
    Tab: "\t",
    ArrowUp: "\x1b[A",
    ArrowDown: "\x1b[B",
    ArrowRight: "\x1b[C",
    ArrowLeft: "\x1b[D"
  };
  document.querySelectorAll(".key-btn[data-key]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      const seq = KEY_SEQ[btn.getAttribute("data-key")];
      if (seq && session) {
        session.sendInput(seq);
      }
      term.focus();
    });
  });

  // ---- boot ---------------------------------------------------------------

  renderStatus({ state: "connecting", code: "", message: "" });
  connect();
  refreshWindows();
  startPolling();
  term.focus();
})();
