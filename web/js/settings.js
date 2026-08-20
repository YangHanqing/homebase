(function () {
  const t = window.homebaseI18n.t;
  const radios = document.querySelectorAll('#access-radios input[name="access"]');
  const rangesEl = document.getElementById("ranges");
  const rangesErr = document.getElementById("ranges-err");
  const saveBtn = document.getElementById("btn-save");
  const saveStatus = document.getElementById("save-status");
  const lanAck = document.getElementById("lan-ack");
  const ackCheckbox = document.getElementById("ack-checkbox");
  const deviceList = document.getElementById("device-list");
  const deviceEmpty = document.getElementById("device-empty");
  let savedAccess = "private";

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
    return "local";
  }

  function hideAck() {
    lanAck.hidden = true;
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
    if (!list || list.length === 0) {
      deviceEmpty.hidden = false;
      return;
    }
    deviceEmpty.hidden = true;
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
    api("/api/settings").then(function (s) {
      savedAccess = s.access || "private";
      radios.forEach(function (r) {
        r.checked = r.value === s.access;
      });
      rangesEl.value = (s.trusted_ranges || []).join("\n");
      hideAck();
      loadDevices();
    }).catch(function (err) {
      if (err.status === 403) {
        document.querySelectorAll(".settings-card, .settings-actions, .settings-ack").forEach(function (el) {
          el.hidden = true;
        });
      } else {
        saveStatus.textContent = err.message;
      }
    });
  }

  radios.forEach(function (r) {
    r.addEventListener("change", hideAck);
  });

  saveBtn.addEventListener("click", function () {
    rangesErr.hidden = true;
    saveStatus.textContent = "";
    const access = currentAccess();
    if (access === "lan" && savedAccess !== "lan" && !ackCheckbox.checked) {
      lanAck.hidden = false;
      rangesErr.textContent = t("s.ackRequired");
      rangesErr.hidden = false;
      return;
    }
    const ranges = rangesEl.value
      .split("\n")
      .map(function (s) { return s.trim(); })
      .filter(function (s) { return s.length > 0; });

    api("/api/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ access: access, trusted_ranges: ranges })
    }).then(function (body) {
      savedAccess = access;
      hideAck();
      if (body && body.restart_required) {
        saveStatus.textContent = t("s.saved");
      } else {
        saveStatus.textContent = t("s.save");
      }
    }).catch(function (err) {
      if (err.status === 403) {
        saveStatus.textContent = t("s.loopbackOnly");
        return;
      }
      rangesErr.textContent = err.message;
      rangesErr.hidden = false;
    });
  });

  document.addEventListener("homebase:lang-changed", function () {
    if (deviceList.children.length > 0 || !deviceEmpty.hidden) {
      loadDevices();
    }
  });

  document.addEventListener("DOMContentLoaded", load);
})();
