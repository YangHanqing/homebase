(function () {
  const appEl = document.getElementById("app");
  const projListEl = document.getElementById("proj-list");
  const listMsg = document.getElementById("list-msg");
  const termWrap = document.getElementById("term-wrap");
  const paneStatus = document.getElementById("pane-status");
  const paneStatusLine = document.getElementById("pane-status-line");
  const paneStatusHelp = document.getElementById("pane-status-help");
  const navToggle = document.getElementById("nav-toggle");
  const scrim = document.getElementById("scrim");
  const chromeHost = document.getElementById("chrome-host");
  const chromeLed = document.getElementById("chrome-led");
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

  // One project is "attached": the single terminal/WebSocket follows it, and
  // "" means the legacy singleton session (the ungrouped bucket). Every
  // other tracked project still lists its windows via the control channel —
  // that part needs no attached PTY — but only the attached one is what the
  // terminal actually shows.
  let currentProject = "";
  let projects = [];               // [{id, name, path}]
  let sections = {};                // project id (or "") -> {windows, clients}
  let session = null;
  let pollTimer = null;
  let inFlight = false;
  let lastStatus = { state: "connecting" };

  const COLLAPSE_KEY = "homebase.collapsedProjects";
  function loadCollapsed() {
    try {
      const raw = JSON.parse(localStorage.getItem(COLLAPSE_KEY));
      return Array.isArray(raw) ? raw : [];
    } catch (e) {
      return [];
    }
  }
  let collapsed = loadCollapsed();
  function isCollapsed(id) {
    return collapsed.indexOf(id) >= 0;
  }
  function toggleCollapsed(id) {
    const i = collapsed.indexOf(id);
    if (i >= 0) {
      collapsed.splice(i, 1);
    } else {
      collapsed.push(id);
    }
    try {
      localStorage.setItem(COLLAPSE_KEY, JSON.stringify(collapsed));
    } catch (e) { /* private mode: collapse state lasts this page only */ }
  }

  // Remembers which window was showing in the terminal, so a page reload
  // lands back on it instead of always defaulting to Ungrouped. Read once at
  // boot (restoreLastFocus); written every time refreshWindows learns which
  // window is actually attached-and-current, which covers a click, a new
  // window, and tmux's own window becoming current for any other reason,
  // without every call site having to remember to save it.
  const LAST_FOCUS_KEY = "homebase.lastFocus";
  function loadLastFocus() {
    try {
      const raw = JSON.parse(localStorage.getItem(LAST_FOCUS_KEY));
      if (raw && typeof raw.project === "string" && Number.isInteger(raw.index)) {
        return raw;
      }
    } catch (e) { /* private mode, or nothing saved yet */ }
    return null;
  }
  function saveLastFocus(project, index) {
    try {
      localStorage.setItem(LAST_FOCUS_KEY, JSON.stringify({ project: project || "", index: index }));
    } catch (e) { /* private mode: this page only, nothing to restore next time */ }
  }

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
    chromeLed.className = "led " + (st.state || "");
    // The sidebar's own status dot lives on whichever project is attached
    // (renderSection), not here -- it needs a re-render of that row, not a
    // style update, and it is already a no-op when nothing has been fetched
    // yet (renderSection tolerates a missing section).
    renderSidebar();

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
    const s = sections[currentProject];
    if (!s) {
      return null;
    }
    for (let i = 0; i < s.windows.length; i++) {
      if (s.windows[i].active) {
        return s.windows[i];
      }
    }
    return null;
  }

  // ---- the single session -------------------------------------------------

  // Switches which tmux session the one terminal/WebSocket is attached to.
  // A click on a window already in the attached project is not a project
  // switch at all, so callers that just want "make sure we're looking at
  // this project" should go through ensureConnected instead.
  function connect(project) {
    currentProject = project || "";
    if (session) {
      session.dispose();
      session = null;
    }
    term.reset();
    session = new HomebaseSession({
      term: term,
      project: currentProject,
      onStatus: function (st) {
        renderStatus(st);
        if (st.state === "connected") {
          // The session may have just been created by this very connect.
          refreshAll();
        }
      }
    });
    const size = homebaseFitSize(fit, term);
    session.setSize(size.cols, size.rows);
    session.connect();
  }

  function ensureConnected(project) {
    project = project || "";
    if (project !== currentProject) {
      connect(project);
    }
  }

  // ---- tmux windows -------------------------------------------------------

  // Thresholds for the activity dot, in seconds since a window last produced
  // output. The list polls every POLL_MS (3s), so anything under a couple of
  // poll intervals is indistinguishable from "right now".
  //
  // BUSY_S is 20s because that is comfortably longer than the gaps a working
  // agent leaves: a model thinking between tool calls, a compile step, a test
  // run printing nothing for a moment. Shorter and the dot strobes on and off
  // while the window is plainly still working.
  //
  // RECENT_S is 3 minutes: long enough that a window which finished while the
  // phone was in a pocket still says so when it comes out, short enough that
  // the shell you last typed in an hour ago does not claim to be busy.
  const BUSY_S = 20;
  const RECENT_S = 180;

  // serverNow is the clock reading the server sent with the last window list.
  // Window.activity is on the server's clock too, so the age is the
  // difference between two readings of the same clock — never Date.now(),
  // which on a phone that has been asleep can be minutes out and would make
  // every window look either frantic or dead.
  let serverNow = 0;

  // activityState never removes the dot from the layout, only recolors it:
  // the timestamp ticks every poll, and a badge appearing and disappearing
  // would shove the window name sideways three times a minute.
  function activityState(w) {
    if (w.bell) {
      return "bell";
    }
    if (!w.activity || !serverNow) {
      return "quiet";
    }
    const age = serverNow - w.activity;
    if (age <= BUSY_S) {
      return "busy";
    }
    if (age <= RECENT_S) {
      return "recent";
    }
    return "quiet";
  }

  // renderSidebar draws every section: the fixed ungrouped bucket (the
  // legacy singleton session, kept for zero-migration compatibility) first,
  // then one section per tracked project, in the order projects.json has
  // them.
  function renderSidebar() {
    projListEl.innerHTML = "";
    renderSection("", t("sidebar.ungrouped"), null);
    projects.forEach(function (p) {
      renderSection(p.id, p.name, p);
    });
    syncChrome();
  }

  // The connection led + client count belong to whichever project is
  // attached -- there is exactly one terminal/WebSocket for the whole page,
  // so showing this on every project read as each of them having its own
  // status. Only the attached section's header carries it.
  function attachedIndicator() {
    const wrap = el("span", "proj-attached");
    const dot = el("span", "led " + (lastStatus.state || ""));
    dot.setAttribute("aria-hidden", "true");
    dot.title = lastStatus.state === "connected" ? "" : (t(LABEL[lastStatus.state]) || lastStatus.state || "");
    wrap.appendChild(dot);
    // Only worth a badge once someone besides the viewer themself is
    // attached -- "1 watching" is always true and just adds width to a row
    // that already has the dot for "connected". The badge is a bare count,
    // not a sentence; the full "N watching" phrasing lives in the tooltip.
    const count = (sections[currentProject] || {}).clients;
    if (count > 1) {
      const c = el("span", "proj-attached-clients is-crowded");
      c.textContent = t("sidebar.clients", { count: count });
      c.title = t("sidebar.clientsHint", { count: count });
      wrap.appendChild(c);
    }
    return wrap;
  }

  function renderSection(id, label, project) {
    const s = sections[id] || { windows: [] };
    const collapsedNow = isCollapsed(id);

    const wrap = el("div", "proj-section");
    const head = el("div", "proj-head");
    const arrow = el("span", "proj-collapse");
    arrow.setAttribute("aria-hidden", "true");
    arrow.textContent = collapsedNow ? "▸" : "▾";
    const nameEl = el("div", "proj-name");
    nameEl.textContent = label;
    if (project) {
      nameEl.title = project.path;
    }

    const actions = el("div", "proj-actions");
    // Delete sits to the left of "+", revealed on hover/focus by taking up
    // no layout space at all (proj-del below, not the opacity-only
    // hover-reveal a window row's actions use) -- otherwise its reserved
    // slot would push "+" left of its resting place even while hidden. "+"
    // (new window) is the one frequent action here, so it stays last: a
    // normal-weight, always-visible button pinned at the row's right edge.
    if (project) {
      const delBtn = el("button", "btn-tiny proj-del");
      delBtn.type = "button";
      delBtn.innerHTML = window.homebaseIcons.markup("x");
      delBtn.title = t("project.delete");
      delBtn.setAttribute("aria-label", t("project.deleteAria", { name: label }));
      delBtn.addEventListener("click", function (ev) {
        ev.stopPropagation();
        deleteProject(project);
      });
      actions.appendChild(delBtn);
    }
    const addBtn = el("button", "proj-add");
    addBtn.type = "button";
    addBtn.textContent = "+";
    addBtn.title = t("sidebar.new");
    addBtn.setAttribute("aria-label", t("project.newWindowAria", { name: label }));
    addBtn.addEventListener("click", function (ev) {
      ev.stopPropagation();
      newWindow(id);
    });
    actions.appendChild(addBtn);

    head.appendChild(arrow);
    head.appendChild(nameEl);
    if (id === currentProject) {
      head.appendChild(attachedIndicator());
    }
    head.appendChild(actions);
    head.setAttribute("role", "button");
    head.setAttribute("aria-expanded", collapsedNow ? "false" : "true");
    head.setAttribute("aria-label", collapsedNow ? t("sidebar.expand") : t("sidebar.collapse"));
    head.addEventListener("click", function () {
      toggleCollapsed(id);
      renderSidebar();
    });
    wrap.appendChild(head);

    if (!collapsedNow) {
      if (s.err) {
        const msg = el("p", "list-msg");
        msg.textContent = s.err;
        wrap.appendChild(msg);
      } else if (!s.windows.length) {
        const msg = el("p", "list-msg");
        msg.textContent = t("list.empty");
        wrap.appendChild(msg);
      } else {
        const ul = el("ul", "list");
        // Only the ungrouped bucket keeps the legacy singleton session's
        // "cannot kill the last window" rule (see AGENT.md hard constraint
        // 3). A project's tmux session has no such guard: its existence
        // lives in projects.json, not in keeping a window open, so closing
        // its last window is allowed and simply ends that session.
        const onlyOne = id === "" && s.windows.length <= 1;
        s.windows.forEach(function (w) {
          ul.appendChild(renderWindowRow(id, w, onlyOne));
        });
        wrap.appendChild(ul);
      }
    }
    projListEl.appendChild(wrap);
  }

  function renderWindowRow(project, w, onlyOne) {
    // tmux tracks a "current window" per session independently, so every
    // project's own section has one. Highlighting all of them read as
    // several windows being "active" at once; only the window actually
    // showing in the one terminal -- the attached project's current window
    // -- should look selected.
    const attached = project === currentProject && w.active;
    const li = el("li", "plate" + (attached ? " is-active" : ""));

    const main = el("button", "plate-main");
    main.type = "button";
    const state = activityState(w);
    const stateLabel = t("window.act." + state);
    const dotEl = el("span", "plate-dot is-" + state);
    dotEl.setAttribute("aria-hidden", "true");
    dotEl.title = stateLabel;
    const nameEl = el("div", "plate-name");
    nameEl.textContent = w.name;
    const idxEl = el("div", "plate-index");
    idxEl.textContent = w.index;
    main.appendChild(dotEl);
    main.appendChild(idxEl);
    main.appendChild(nameEl);
    // The dot is decorative, so the state has to reach a screen reader
    // through the button's own name.
    main.setAttribute("aria-label", t("window.selectAria", {
      index: w.index,
      name: w.name,
      state: stateLabel
    }));
    main.addEventListener("click", function () {
      selectWindow(project, w.index);
    });

    const actions = el("div", "plate-actions hover-reveal");
    const renameBtn = el("button", "btn-tiny");
    renameBtn.type = "button";
    renameBtn.textContent = "✎";
    renameBtn.title = t("window.rename");
    renameBtn.setAttribute("aria-label", t("window.renameAria", { name: w.name }));
    renameBtn.addEventListener("click", function () {
      openRename(project, w);
    });
    const killBtn = el("button", "btn-tiny");
    killBtn.type = "button";
    killBtn.textContent = "×";
    killBtn.title = onlyOne ? t("window.closeLastTitle") : t("window.closeTitle");
    killBtn.setAttribute("aria-label", t("window.closeAria", { name: w.name }));
    killBtn.disabled = onlyOne;
    killBtn.addEventListener("click", function () {
      killWindow(project, w.index);
    });
    actions.appendChild(renameBtn);
    actions.appendChild(killBtn);

    li.appendChild(main);
    li.appendChild(actions);
    return li;
  }

  function showListMsg(text) {
    listMsg.textContent = text || "";
    listMsg.hidden = !text;
  }

  function windowsPath(project, suffix) {
    let p = "/api/windows" + (suffix || "");
    if (project) {
      p += "?project=" + encodeURIComponent(project);
    }
    return p;
  }

  function scrollPath() {
    return "/api/scroll" + (currentProject ? "?project=" + encodeURIComponent(currentProject) : "");
  }

  function sectionIds() {
    return [""].concat(projects.map(function (p) { return p.id; }));
  }

  function refreshProjects() {
    return api("/api/projects").then(function (body) {
      projects = (body && body.projects) || [];
    }).catch(function () {
      // Keep whatever the sidebar already has; the next poll tries again.
    });
  }

  function refreshWindows() {
    if (inFlight) {
      return Promise.resolve();
    }
    inFlight = true;
    const ids = sectionIds();
    return Promise.all(ids.map(function (id) {
      return api(windowsPath(id)).then(function (body) {
        // Sections are fetched back-to-back in one poll tick, so their "now"
        // readings differ by at most the request latency; the sidebar only
        // needs one to keep the activity age off the browser's own clock
        // (see activityState below and AGENT.md's server-clock rule).
        serverNow = (body && body.now) || serverNow;
        sections[id] = { windows: (body && body.windows) || [], clients: (body && body.clients) || 0, err: null };
      }).catch(function (err) {
        sections[id] = { windows: (sections[id] && sections[id].windows) || [], clients: 0, err: err.message };
      });
    })).then(function () {
      renderSidebar();
      const active = activeWindow();
      if (active) {
        saveLastFocus(currentProject, active.index);
      }
    }).finally(function () {
      inFlight = false;
    });
  }

  function refreshAll() {
    return refreshProjects().then(refreshWindows);
  }

  function act(promise) {
    return promise.then(function () {
      return refreshWindows();
    }).catch(function (err) {
      showListMsg(err.message);
    });
  }

  function selectWindow(project, index) {
    // Optimistic: tmux redraws through the PTY faster than we can re-list.
    const s = sections[project];
    if (s) {
      s.windows.forEach(function (w) { w.active = w.index === index; });
      renderSidebar();
    }
    if (isMobile()) {
      setNavOpen(false);
    }
    ensureConnected(project);
    act(api(windowsPath(project, "/" + index), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ active: true })
    }));
  }

  function newWindow(project) {
    ensureConnected(project);
    act(api(windowsPath(project), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ dir: homebasePref("newWindowDir", "same") })
    }));
  }

  function killWindow(project, index) {
    // A project session has no "last window" guard (AGENT.md hard constraint
    // 3): closing this one may end the whole session on purpose. If that
    // session is the one attached, the normal WS reconnect (Model A's
    // backoff loop) would otherwise recreate it seconds later via
    // "new-session -A", silently undoing the close the user just asked for.
    const s = sections[project];
    const killingLast = !!s && s.windows.length <= 1;
    const req = api(windowsPath(project, "/" + index), { method: "DELETE" });
    act(req);
    if (killingLast && project === currentProject && project !== "") {
      req.then(function () {
        connect("");
      }).catch(function () {});
    }
  }

  // ---- projects -------------------------------------------------------------

  const projectDeleteConfirm = document.getElementById("project-delete-confirm");
  const projectDeleteBody = document.getElementById("project-delete-body");
  const btnProjectDeleteConfirm = document.getElementById("btn-project-delete-confirm");
  const btnProjectDeleteCancel = document.getElementById("btn-project-delete-cancel");
  let pendingDeleteProject = null;

  function showProjectDeleteConfirm(show) {
    projectDeleteConfirm.hidden = !show;
    if (!show) {
      pendingDeleteProject = null;
    }
  }

  function deleteProject(p) {
    pendingDeleteProject = p;
    projectDeleteBody.textContent = t("project.deleteConfirm", { name: p.name });
    showProjectDeleteConfirm(true);
  }

  btnProjectDeleteCancel.addEventListener("click", function () {
    showProjectDeleteConfirm(false);
  });
  projectDeleteConfirm.addEventListener("click", function (ev) {
    if (ev.target === projectDeleteConfirm) {
      showProjectDeleteConfirm(false);
    }
  });
  btnProjectDeleteConfirm.addEventListener("click", function () {
    const p = pendingDeleteProject;
    if (!p) {
      return;
    }
    showProjectDeleteConfirm(false);
    api("/api/projects/" + encodeURIComponent(p.id), { method: "DELETE" }).then(function () {
      delete sections[p.id];
      if (currentProject === p.id) {
        connect("");
      }
      return refreshAll();
    }).catch(function (err) {
      showListMsg(err.message);
    });
  });

  const projectModal = document.getElementById("project-modal");
  const projectForm = document.getElementById("project-form");
  const projectFormErr = document.getElementById("project-form-err");
  const projectPathInput = document.getElementById("p-path");

  document.getElementById("btn-project-add").addEventListener("click", function () {
    projectForm.reset();
    projectFormErr.hidden = true;
    projectModal.hidden = false;
    projectPathInput.focus();
  });
  document.getElementById("btn-project-cancel").addEventListener("click", function () {
    projectModal.hidden = true;
  });
  projectModal.addEventListener("click", function (ev) {
    if (ev.target === projectModal) {
      projectModal.hidden = true;
    }
  });
  projectForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    projectFormErr.hidden = true;
    const path = projectPathInput.value.trim();
    if (!path) {
      projectFormErr.textContent = t("project.add.err.required");
      projectFormErr.hidden = false;
      return;
    }
    api("/api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: path })
    }).then(function () {
      projectModal.hidden = true;
      return refreshAll();
    }).catch(function (err) {
      projectFormErr.textContent = err.message;
      projectFormErr.hidden = false;
    });
  });

  // ---- directory picker -----------------------------------------------------

  const browseModal = document.getElementById("browse-modal");
  const browseList = document.getElementById("browse-list");
  const browsePathEl = document.getElementById("browse-path");
  const browseFormErr = document.getElementById("browse-form-err");
  let browseCurrent = "";

  function loadBrowse(path) {
    browseFormErr.hidden = true;
    const q = path ? "?path=" + encodeURIComponent(path) : "";
    api("/api/browse" + q).then(function (body) {
      browseCurrent = body.path;
      browsePathEl.textContent = browseCurrent;
      browseList.innerHTML = "";
      if (body.parent) {
        const up = el("li", "browse-item browse-up");
        up.textContent = t("browse.up");
        up.addEventListener("click", function () { loadBrowse(body.parent); });
        browseList.appendChild(up);
      }
      const dirs = body.entries || [];
      if (!dirs.length) {
        const empty = el("li", "browse-empty");
        empty.textContent = t("browse.empty");
        browseList.appendChild(empty);
      }
      dirs.forEach(function (entry) {
        const li = el("li", "browse-item");
        li.textContent = entry.name;
        li.addEventListener("click", function () { loadBrowse(entry.path); });
        browseList.appendChild(li);
      });
    }).catch(function (err) {
      browseFormErr.textContent = err.message;
      browseFormErr.hidden = false;
    });
  }

  document.getElementById("btn-project-browse").addEventListener("click", function () {
    browseModal.hidden = false;
    loadBrowse(projectPathInput.value.trim());
  });
  document.getElementById("btn-browse-cancel").addEventListener("click", function () {
    browseModal.hidden = true;
  });
  document.getElementById("btn-browse-select").addEventListener("click", function () {
    projectPathInput.value = browseCurrent;
    browseModal.hidden = true;
  });
  browseModal.addEventListener("click", function (ev) {
    if (ev.target === browseModal) {
      browseModal.hidden = true;
    }
  });

  // ---- settings modal ---------------------------------------------------------
  // Settings used to be a full navigation to /settings.html; it now opens in a
  // modal instead, with settings.html loaded lazily into an iframe on first
  // open so a session that never touches Settings never pays for it.

  const settingsModal = document.getElementById("settings-modal");
  const settingsFrame = document.getElementById("settings-frame");
  const btnSettingsClose = document.getElementById("btn-settings-close");
  btnSettingsClose.innerHTML = window.homebaseIcons.markup("x");

  function setSettingsOpen(open) {
    settingsModal.hidden = !open;
    if (open && !settingsFrame.src) {
      settingsFrame.src = "/settings.html";
    }
  }

  document.getElementById("btn-settings-open").addEventListener("click", function () {
    setSettingsOpen(true);
  });
  btnSettingsClose.addEventListener("click", function () {
    setSettingsOpen(false);
  });
  settingsModal.addEventListener("click", function (ev) {
    if (ev.target === settingsModal) {
      setSettingsOpen(false);
    }
  });
  // settings.html runs Escape-to-close itself when embedded (see its own
  // keydown handler), since Escape inside the iframe never reaches this
  // document's listener.
  window.addEventListener("message", function (ev) {
    if (ev.origin === window.location.origin &&
        ev.source === settingsFrame.contentWindow &&
        ev.data === "homebase:settings-close") {
      setSettingsOpen(false);
    }
  });

  // ---- rename modal ---------------------------------------------------------

  function openRename(project, w) {
    renameForm.reset();
    renameFormErr.hidden = true;
    document.getElementById("r-project").value = project || "";
    document.getElementById("r-index").value = w.index;
    document.getElementById("r-name").value = w.name;
    renameModal.hidden = false;
    document.getElementById("r-name").focus();
    document.getElementById("r-name").select();
  }

  renameForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    renameFormErr.hidden = true;
    const project = document.getElementById("r-project").value;
    const index = document.getElementById("r-index").value;
    const name = document.getElementById("r-name").value;
    api(windowsPath(project, "/" + encodeURIComponent(index)), {
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
    if (!projectDeleteConfirm.hidden) {
      showProjectDeleteConfirm(false);
      return;
    }
    if (!settingsModal.hidden) {
      setSettingsOpen(false);
      return;
    }
    if (appEl.classList.contains("is-nav-open")) {
      setNavOpen(false);
    }
  });
  document.addEventListener("homebase:lang-changed", function () {
    renderSidebar();
    renderStatus(lastStatus);
    renderThemeButtons();
  });

  function themeIcon(theme) {
    if (theme === "light") {
      return "sun";
    }
    if (theme === "dark") {
      return "moon";
    }
    return "monitor";
  }

  function renderThemeButtons() {
    const theme = window.homebasePrefs.get("theme");
    const name = t("theme." + theme);
    const title = t("theme.title", { name: name });
    document.querySelectorAll("[data-theme-cycle]").forEach(function (btn) {
      btn.innerHTML = window.homebaseIcons.markup(themeIcon(theme));
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
      refreshAll();
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
    refreshAll();
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
    api(scrollPath(), {
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
    return api(scrollPath(), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ lines: 0 })
    }).catch(function () {});
  }

  // ---- boot ---------------------------------------------------------------

  // Best-effort restore of whatever window was showing last time: reattach
  // its project, and if the window itself still exists, make it current
  // again. Either check can fail (the project was deleted, the window was
  // closed) and both are fine to just skip -- whatever the session's own
  // current window already is stays current, per the ask ("except when it's
  // already been closed").
  function restoreLastFocus(saved) {
    if (!saved) {
      return;
    }
    const projectExists = saved.project === "" || projects.some(function (p) { return p.id === saved.project; });
    if (!projectExists) {
      if (currentProject !== "") {
        connect("");
      }
      return;
    }
    if (saved.project !== currentProject) {
      // The optimistic boot-time connect (below) targeted this same project
      // already in the common case; this only runs if that guess differed.
      connect(saved.project);
    }
    const s = sections[saved.project];
    if (!s || !s.windows.some(function (w) { return w.index === saved.index; })) {
      return; // closed since last time -- nothing to restore
    }
    if (!s.windows.some(function (w) { return w.index === saved.index && w.active; })) {
      selectWindow(saved.project, saved.index);
    }
  }

  const savedFocus = loadLastFocus();

  renderStatus({ state: "connecting", code: "", message: "" });
  // Connect immediately to the best guess (page load must not wait on a
  // round-trip before showing a shell); refreshAll's result is what lets
  // restoreLastFocus tell whether that guess, and the saved window inside
  // it, are still valid.
  connect(savedFocus ? savedFocus.project : "");
  refreshAll().then(function () {
    restoreLastFocus(savedFocus);
  });
  startPolling();
  term.focus();
})();
