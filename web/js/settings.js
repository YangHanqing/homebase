(function () {
  const t = window.homebaseI18n.t;
  const radios = document.querySelectorAll('#access-radios input[name="access"]');
  const rangesEl = document.getElementById("ranges");
  const rangesCard = document.getElementById("ranges-card");
  const rangesErr = document.getElementById("ranges-err");
  const saveBtn = document.getElementById("btn-save");
  const saveStatus = document.getElementById("save-status");
  const confirmEl = document.getElementById("save-confirm");
  const confirmSave = document.getElementById("btn-confirm-save");
  const confirmCancel = document.getElementById("btn-confirm-cancel");
  const deviceList = document.getElementById("device-list");
  const deviceEmpty = document.getElementById("device-empty");
  const revokeAllBtn = document.getElementById("btn-revoke-all");
  const revokeConfirm = document.getElementById("revoke-confirm");
  const confirmRevoke = document.getElementById("btn-confirm-revoke");
  const revokeCancel = document.getElementById("btn-revoke-cancel");
  const loopbackNotice = document.getElementById("loopback-notice");
  const securityPanel = document.getElementById("panel-security");
  const securityControls = document.getElementById("security-controls");
  const settingsVersion = document.getElementById("settings-version");
  const embedded = window.self !== window.top;
  const maxRanges = 5;
  // Baseline to diff against for the Save button's dirty highlight.
  let savedAccess = null;
  let savedRangesRaw = null;

  // ---- tabs ---------------------------------------------------------------
  // Preferences are local-only; Security is server config the API gates to
  // 127.0.0.1. They share a page but never share a control.
  const tabs = document.querySelectorAll(".settings-tab");
  const panels = {
    appearance: document.getElementById("panel-appearance"),
    security: securityPanel
  };

  function selectTab(name) {
    tabs.forEach(function (b) {
      const on = b.getAttribute("data-tab") === name;
      b.classList.toggle("is-active", on);
      b.setAttribute("aria-selected", on ? "true" : "false");
    });
    Object.keys(panels).forEach(function (k) {
      if (panels[k]) {
        panels[k].hidden = k !== name;
      }
    });
    try { localStorage.setItem("homebase.settingsTab", name); } catch (e) { /* private mode */ }
  }

  tabs.forEach(function (b) {
    b.addEventListener("click", function () {
      selectTab(b.getAttribute("data-tab"));
    });
  });

  // ---- appearance preferences --------------------------------------------
  const prefs = window.homebasePrefs;
  const langRadios = document.querySelectorAll('#lang-radios input[name="lang"]');
  const copyOnSelect = document.getElementById("pref-copy-on-select");
  const cursorBlink = document.getElementById("pref-cursor-blink");
  const doubleTapTab = document.getElementById("pref-double-tap-tab");
  const fontSize = document.getElementById("pref-font-size");
  const fontSizeOut = document.getElementById("pref-font-size-out");
  const scrollback = document.getElementById("pref-scrollback");
  const newWinDirRadios = document.querySelectorAll('#newwindir-radios input[name="newwindir"]');

  function renderLang() {
    const choice = window.homebaseI18n.pref();
    langRadios.forEach(function (r) { r.checked = r.value === choice; });
  }

  function renderPrefs() {
    const p = prefs.all();
    copyOnSelect.checked = p.copyOnSelect;
    cursorBlink.checked = p.cursorBlink;
    doubleTapTab.checked = p.doubleTapTab;
    fontSize.value = String(p.fontSize);
    fontSizeOut.textContent = String(p.fontSize);
    scrollback.value = String(p.scrollback);
    newWinDirRadios.forEach(function (r) { r.checked = r.value === p.newWindowDir; });
  }

  function bindPrefs() {
    langRadios.forEach(function (r) {
      r.addEventListener("change", function () {
        if (r.checked) {
          window.homebaseI18n.setLang(r.value);
        }
      });
    });
    copyOnSelect.addEventListener("change", function () {
      prefs.set("copyOnSelect", copyOnSelect.checked);
    });
    cursorBlink.addEventListener("change", function () {
      prefs.set("cursorBlink", cursorBlink.checked);
    });
    doubleTapTab.addEventListener("change", function () {
      prefs.set("doubleTapTab", doubleTapTab.checked);
    });
    fontSize.addEventListener("input", function () {
      fontSizeOut.textContent = fontSize.value;
      prefs.set("fontSize", Number(fontSize.value));
    });
    scrollback.addEventListener("change", function () {
      prefs.set("scrollback", Number(scrollback.value));
    });
    newWinDirRadios.forEach(function (r) {
      r.addEventListener("change", function () {
        if (r.checked) {
          prefs.set("newWindowDir", r.value);
        }
      });
    });
    // Another tab (or the terminal page) changed something.
    document.addEventListener("homebase:prefs-changed", renderPrefs);
    document.addEventListener("homebase:lang-changed", renderLang);
  }

  function api(path, opts) {
    return fetch(path, opts).then(function (res) {
      if (res.status === 204) {
        if (!res.ok) {
          const err = new Error(res.statusText);
          err.status = res.status;
          throw err;
        }
        return null;
      }
      return res.json().catch(function () { return null; }).then(function (body) {
        if (!res.ok) {
          const err = new Error((body && body.error) || res.statusText);
          err.status = res.status;
          throw err;
        }
        return body;
      });
    });
  }

  function currentAccess() {
    for (let i = 0; i < radios.length; i++) {
      if (radios[i].checked) {
        return radios[i].value;
      }
    }
    return "private";
  }

  function parsedRanges() {
    return rangesEl.value
      .split("\n")
      .map(function (s) { return s.trim(); })
      .filter(function (s) { return s.length > 0; });
  }

  function syncRangesVisibility() {
    if (rangesCard) {
      rangesCard.hidden = currentAccess() === "lan";
    }
  }

  // The plain, unhighlighted Save button reads as inert, so an edited-but-
  // unsaved Security tab looked identical to a freshly loaded one. Highlight
  // it the moment the form differs from what the server last reported.
  function syncDirty() {
    const dirty = savedAccess !== null &&
      (currentAccess() !== savedAccess || rangesEl.value !== savedRangesRaw);
    saveBtn.classList.toggle("is-dirty", dirty);
  }

  function setBaseline(access, rangesRaw) {
    savedAccess = access;
    savedRangesRaw = rangesRaw;
    syncDirty();
  }

  function showConfirm(show) {
    confirmEl.hidden = !show;
  }

  function showRevokeConfirm(show) {
    revokeConfirm.hidden = !show;
  }

  function delay(ms) {
    return new Promise(function (resolve) { setTimeout(resolve, ms); });
  }

  function waitForHealth(deadline) {
    return fetch("/api/health", { cache: "no-store" }).then(function (res) {
      if (!res.ok) {
        throw new Error("down");
      }
      return res.json();
    }).then(function (body) {
      if (body && body.ok) {
        return;
      }
      throw new Error("down");
    }).catch(function () {
      if (Date.now() > deadline) {
        throw new Error(t("s.restartTimeout"));
      }
      return delay(400).then(function () { return waitForHealth(deadline); });
    });
  }

  function formatDate(iso) {
    if (!iso) {
      return "";
    }
    const d = new Date(iso);
    if (isNaN(d.getTime())) {
      return iso;
    }
    return d.toLocaleString();
  }

  function renderDevices(list) {
    deviceList.textContent = "";
    const empty = !list || list.length === 0;
    deviceEmpty.hidden = !empty;
    if (revokeAllBtn) {
      revokeAllBtn.hidden = empty;
    }
    if (empty) {
      return;
    }
    list.forEach(function (d) {
      const li = document.createElement("li");
      li.className = "device-row";
      const meta = document.createElement("span");
      meta.className = "device-meta";
      const name = document.createElement("strong");
      name.textContent = d.name || d.id;
      const when = document.createElement("small");
      when.textContent = formatDate(d.created_at);
      meta.appendChild(name);
      meta.appendChild(when);
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "btn-tiny btn-danger";
      btn.textContent = t("s.devicesRevoke");
      btn.addEventListener("click", function () {
        api("/api/devices/" + encodeURIComponent(d.id), { method: "DELETE" })
          .then(loadDevices)
          .catch(function (err) {
            saveStatus.textContent = err.message;
          });
      });
      li.appendChild(meta);
      li.appendChild(btn);
      deviceList.appendChild(li);
    });
  }

  function loadDevices() {
    api("/api/devices").then(function (body) {
      renderDevices((body && body.devices) || []);
    }).catch(function (err) {
      if (err.status !== 403) {
        saveStatus.textContent = err.message;
      }
    });
  }

  function load() {
    // Fail closed: stay hidden until the server confirms this browser is
    // actually on 127.0.0.1. A 401 (unpaired) or 403 (paired, but not
    // loopback) both mean "cannot change these", not just the 403 case.
    securityControls.hidden = true;
    if (loopbackNotice) {
      loopbackNotice.hidden = true;
    }
    api("/api/settings").then(function (s) {
      const access = s.access === "lan" ? "lan" : "private";
      radios.forEach(function (r) {
        r.checked = r.value === access;
      });
      rangesEl.value = (s.trusted_ranges || []).join("\n");
      syncRangesVisibility();
      setBaseline(access, rangesEl.value);
      securityControls.hidden = false;
      if (s.version) {
        settingsVersion.textContent = "homebase " + s.version;
      }
      loadDevices();
    }).catch(function (err) {
      if (err.status === 401 || err.status === 403) {
        if (loopbackNotice) {
          loopbackNotice.hidden = false;
        }
      } else {
        saveStatus.textContent = err.message;
      }
    });
  }

  function persist() {
    rangesErr.hidden = true;
    saveStatus.textContent = "";
    const access = currentAccess();
    const ranges = parsedRanges();
    saveBtn.disabled = true;
    confirmSave.disabled = true;
    api("/api/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ access: access, trusted_ranges: ranges })
    }).then(function (body) {
      showConfirm(false);
      setBaseline(access, rangesEl.value);
      if (body && body.restarting) {
        saveStatus.textContent = t("s.restarting");
        return delay(800).then(function () {
          return waitForHealth(Date.now() + 20000);
        }).then(function () {
          window.location.reload();
        });
      }
      saveStatus.textContent = t("s.saved");
    }).catch(function (err) {
      showConfirm(false);
      if (err.status === 403) {
        saveStatus.textContent = t("s.loopbackOnly");
        return;
      }
      rangesErr.textContent = err.message;
      rangesErr.hidden = false;
      if (access === "lan") {
        saveStatus.textContent = err.message;
      }
    }).then(function () {
      saveBtn.disabled = false;
      confirmSave.disabled = false;
    });
  }

  radios.forEach(function (r) {
    r.addEventListener("change", function () {
      syncRangesVisibility();
      syncDirty();
    });
  });
  rangesEl.addEventListener("input", syncDirty);

  saveBtn.addEventListener("click", function () {
    rangesErr.hidden = true;
    saveStatus.textContent = "";
    if (currentAccess() !== "lan") {
      const ranges = parsedRanges();
      if (ranges.length > maxRanges) {
        rangesErr.textContent = t("s.rangesMax");
        rangesErr.hidden = false;
        return;
      }
    }
    showConfirm(true);
  });

  confirmCancel.addEventListener("click", function () {
    showConfirm(false);
  });

  confirmEl.addEventListener("click", function (ev) {
    if (ev.target === confirmEl) {
      showConfirm(false);
    }
  });

  confirmSave.addEventListener("click", persist);

  revokeAllBtn.addEventListener("click", function () {
    showRevokeConfirm(true);
  });
  revokeCancel.addEventListener("click", function () {
    showRevokeConfirm(false);
  });
  revokeConfirm.addEventListener("click", function (ev) {
    if (ev.target === revokeConfirm) {
      showRevokeConfirm(false);
    }
  });
  confirmRevoke.addEventListener("click", function () {
    confirmRevoke.disabled = true;
    api("/api/devices", { method: "DELETE" })
      .then(function () {
        showRevokeConfirm(false);
        return loadDevices();
      })
      .catch(function (err) {
        showRevokeConfirm(false);
        saveStatus.textContent = err.message;
      })
      .then(function () {
        confirmRevoke.disabled = false;
      });
  });

  document.addEventListener("keydown", function (ev) {
    if (ev.key !== "Escape") {
      return;
    }
    if (!confirmEl.hidden) {
      showConfirm(false);
      return;
    }
    if (!revokeConfirm.hidden) {
      showRevokeConfirm(false);
      return;
    }
    // Escape inside the iframe never reaches the parent document's own
    // listener, so the modal chrome that owns the close button has to be
    // told explicitly.
    if (embedded) {
      window.parent.postMessage("homebase:settings-close", window.location.origin);
    }
  });

  document.addEventListener("homebase:lang-changed", function () {
    if (deviceList.children.length > 0 || !deviceEmpty.hidden) {
      loadDevices();
    }
  });

  document.addEventListener("DOMContentLoaded", function () {
    if (embedded) {
      document.body.classList.add("is-embedded");
    }
    let tab = "appearance";
    try {
      const saved = localStorage.getItem("homebase.settingsTab");
      if (saved === "appearance" || saved === "security") {
        tab = saved;
      }
    } catch (e) { /* private mode */ }
    selectTab(tab);
    renderLang();
    renderPrefs();
    bindPrefs();
    load();
  });
})();
