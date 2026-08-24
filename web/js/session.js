function HomebaseSession(opts) {
  this.term = opts.term;
  // Empty means the legacy singleton session; a tracked project's id
  // attaches to its own tmux session instead. See internal/tmux.ProjectSession.
  this.project = opts.project || "";
  this.onStatus = opts.onStatus || function () {};
  this.ws = null;
  this.attempt = 0;
  this.timer = null;
  this.pingTimer = null;
  this.closed = false;
  this.size = { cols: 80, rows: 24 };
  this.state = "disconnected";
  this.encoder = new TextEncoder();
  this.gen = 0;
}

HomebaseSession.prototype.setSize = function (cols, rows) {
  if (cols < 1 || rows < 1) {
    return;
  }
  this.size = { cols: cols, rows: rows };
  this.sendResize();
};

HomebaseSession.prototype.sendResize = function () {
  if (!this.ws || this.ws.readyState !== 1) {
    return;
  }
  this.ws.send(JSON.stringify({
    type: "resize",
    cols: this.size.cols,
    rows: this.size.rows
  }));
};

HomebaseSession.prototype.connect = function () {
  if (this.closed) {
    return;
  }
  this.clearTimer();
  this.gen += 1;
  const gen = this.gen;
  if (this.ws) {
    try { this.ws.close(); } catch (e) {}
    this.ws = null;
  }
  this.setState("connecting", "", "");
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const url = proto + "//" + location.host + "/ws" +
    (this.project ? "?project=" + encodeURIComponent(this.project) : "");
  const ws = new WebSocket(url);
  ws.binaryType = "arraybuffer";
  this.ws = ws;
  const self = this;

  ws.onopen = function () {
    if (gen !== self.gen) {
      return;
    }
    self.sendResize();
  };

  ws.onmessage = function (ev) {
    if (gen !== self.gen) {
      return;
    }
    if (typeof ev.data === "string") {
      self.onControl(ev.data);
      return;
    }
    self.term.write(new Uint8Array(ev.data));
  };

  ws.onclose = function (ev) {
    if (gen !== self.gen) {
      return;
    }
    self.ws = null;
    self.stopPing();
    if (self.closed) {
      return;
    }
    // Server may have already sent a classified error; don't replace it
    // with a generic "disconnected" or the real cause never shows.
    if (self.state !== "error") {
      self.setState("disconnected", "ws_closed", ev.reason || "");
    }
    self.scheduleReconnect();
  };

  ws.onerror = function () {
    // onclose follows
  };
};

HomebaseSession.prototype.sendInput = function (data) {
  if (!this.ws || this.ws.readyState !== 1) {
    return;
  }
  this.ws.send(this.encoder.encode(data));
};

HomebaseSession.prototype.onControl = function (raw) {
  let msg;
  try {
    msg = JSON.parse(raw);
  } catch (e) {
    return;
  }
  if (msg.type === "pong") {
    return;
  }
  if (msg.type !== "status") {
    return;
  }
  this.setState(msg.state || "error", msg.code || "", msg.message || "");
  if (msg.state === "connected") {
    this.attempt = 0;
    this.startPing();
    this.sendResize();
  }
};

HomebaseSession.prototype.setState = function (state, code, message) {
  this.state = state;
  this.onStatus({ state: state, code: code || "", message: message || "" });
};

HomebaseSession.prototype.scheduleReconnect = function () {
  if (this.closed) {
    return;
  }
  const delay = homebaseNextDelayMs(this.attempt);
  this.attempt += 1;
  const self = this;
  this.timer = setTimeout(function () {
    self.term.reset();
    self.connect();
  }, delay);
};

HomebaseSession.prototype.wake = function () {
  if (this.closed) {
    return;
  }
  if (this.ws && (this.ws.readyState === 0 || this.ws.readyState === 1)) {
    return;
  }
  this.clearTimer();
  this.term.reset();
  this.connect();
};

HomebaseSession.prototype.startPing = function () {
  this.stopPing();
  const self = this;
  this.pingTimer = setInterval(function () {
    if (self.ws && self.ws.readyState === 1) {
      self.ws.send(JSON.stringify({ type: "ping" }));
    }
  }, 20000);
};

HomebaseSession.prototype.stopPing = function () {
  if (this.pingTimer) {
    clearInterval(this.pingTimer);
    this.pingTimer = null;
  }
};

HomebaseSession.prototype.clearTimer = function () {
  if (this.timer) {
    clearTimeout(this.timer);
    this.timer = null;
  }
};

HomebaseSession.prototype.dispose = function () {
  this.closed = true;
  this.gen += 1;
  this.clearTimer();
  this.stopPing();
  if (this.ws) {
    try { this.ws.close(); } catch (e) {}
    this.ws = null;
  }
};
