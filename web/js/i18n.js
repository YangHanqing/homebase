// Minimal i18n: a flat-ish dict per language, applied via data-i18n /
// data-i18n-title / data-i18n-aria / data-i18n-placeholder attributes, plus a
// t(key) helper for strings built up in JS. English is the default; the
// choice is remembered in localStorage and otherwise guessed from the
// browser's language.
(function () {
  const DICT = {
    en: {
      "aria.openWindows": "Open window list",
      "aria.closeWindows": "Close window list",
      "sidebar.windows": "Windows",
      "sidebar.new": "+ New",
      "settings.title": "Settings",
      "rename.title": "Rename window",
      "rename.hint": "A renamed window stops following the running command's own name.",
      "rename.name": "Name",
      "common.cancel": "Cancel",
      "common.save": "Save",
      "window.rename": "Rename",
      "window.renameAria": "Rename window {name}",
      "window.close": "Close",
      "window.closeAria": "Close window {name}",
      "window.closeLastTitle": "This is the last window; closing it would end the session",
      "window.closeTitle": "Close",
      "list.empty": "No windows yet — one appears as soon as you connect.",
      "status.connected": "Connected",
      "status.connecting": "Connecting",
      "status.disconnected": "Disconnected · retrying",
      "status.error": "Error",
      "help.enotmux": "tmux is not installed on this machine. Install it and reconnect to start a new session.",
      "help.pty_spawn": "Could not start the local tmux session. Check the Homebase process log.",
      "help.ws_closed": "",

      "s.back": "← Back",
      "s.title": "Settings",
      "s.loopbackOnly": "Settings can only be viewed and changed from a browser running on this machine (127.0.0.1). Open this page locally to make changes.",
      "s.httpTitle": "This runs over plain HTTP, not HTTPS",
      "s.httpBody": "Nothing encrypts the traffic between your browser and Homebase. Whether that's fine depends entirely on the network you expose it to:",
      "s.httpLocalLabel": "On 127.0.0.1 (this machine only):",
      "s.httpLocalBody": "safe — traffic never leaves the machine.",
      "s.httpTrustedLabel": "On a trusted range below:",
      "s.httpTrustedBody": "safe only if that range really is an already-encrypted overlay network (Tailscale, WireGuard, Headscale, ZeroTier, ...). Homebase cannot verify this — it's your own assertion.",
      "s.httpLanLabel": "On any other network (LAN, public/hotel/office Wi-Fi):",
      "s.httpLanBody": "do not enable this. Anyone else on that network could potentially intercept your session and gain full control of this machine — the same level of access as sitting at its keyboard.",
      "s.accessTitle": "Access",
      "s.accessHint": "After saving, run homebase restart on this machine.",
      "s.accessLocal": "Local only",
      "s.accessLocalBody": "127.0.0.1 only. Nothing leaves this machine. Default and safest.",
      "s.accessPrivate": "Trusted range",
      "s.accessPrivateBody": "Binds an address inside the trusted ranges below (e.g. your Tailscale/WireGuard IP).",
      "s.accessLan": "Local network (risk)",
      "s.accessLanBody": "Binds 0.0.0.0. Plain HTTP, reachable by anyone on the network. Only for networks you fully trust.",
      "s.rangesTitle": "Trusted ranges",
      "s.rangesHint": "One CIDR or IP per line. These are addresses you assert are already encrypted by something else (a VPN/overlay network). Examples:",
      "s.save": "Save",
      "s.saved": "Saved. On this machine, run:  homebase restart",
      "s.ackLan": "I understand this exposes Homebase over plain HTTP to my local network, and that anyone on that network could gain full control of this machine.",
      "s.ackRequired": "Please check the box above to confirm you understand the risk.",
      "s.devicesTitle": "Paired devices",
      "s.devicesHint": "Browsers that can reach Homebase when Access is not Local only. Revoking logs that device out on the next request.",
      "s.devicesEmpty": "No devices paired yet. After switching Access off Local only, run homebase pair on this machine.",
      "s.devicesRevoke": "Revoke"
    },
    zh: {
      "aria.openWindows": "打开窗口列表",
      "aria.closeWindows": "关闭窗口列表",
      "sidebar.windows": "窗口",
      "sidebar.new": "＋ 新建",
      "settings.title": "设置",
      "rename.title": "窗口改名",
      "rename.hint": "改过名的窗口不再跟着当前命令自动改名。",
      "rename.name": "名称",
      "common.cancel": "取消",
      "common.save": "保存",
      "window.rename": "改名",
      "window.renameAria": "给窗口 {name} 改名",
      "window.close": "删除",
      "window.closeAria": "删除窗口 {name}",
      "window.closeLastTitle": "这是最后一个窗口，删掉会连 session 一起没了",
      "window.closeTitle": "删除",
      "list.empty": "还没有窗口，连上就会有一个。",
      "status.connected": "已连接",
      "status.connecting": "连接中",
      "status.disconnected": "已断开 · 重试中",
      "status.error": "出错",
      "help.enotmux": "这台机器上没装 tmux。装好后重连即可开始新 session。",
      "help.pty_spawn": "本机 tmux/pty 没拉起来。请查看 Homebase 进程日志。",
      "help.ws_closed": "",

      "s.back": "← 返回",
      "s.title": "设置",
      "s.loopbackOnly": "设置只能在本机（127.0.0.1）打开的浏览器中查看和修改。请在本机打开这个页面进行修改。",
      "s.httpTitle": "本服务使用明文 HTTP，没有加密",
      "s.httpBody": "浏览器和 Homebase 之间的数据不会被加密。是否安全完全取决于你把它暴露在哪个网络上：",
      "s.httpLocalLabel": "在 127.0.0.1（仅本机）访问：",
      "s.httpLocalBody": "安全——数据不会离开这台电脑。",
      "s.httpTrustedLabel": "在下方“信任网段”内访问：",
      "s.httpTrustedBody": "只有当该网段确实是已经加密的 VPN/Overlay 网络（Tailscale、WireGuard、Headscale、ZeroTier 等）时才安全。Homebase 无法验证这一点——这是你自己的声明。",
      "s.httpLanLabel": "在其他任何网络（局域网、公共/酒店/办公室 Wi-Fi）：",
      "s.httpLanBody": "请不要开启。同一网络下的其他人有可能截获你的会话，从而完全控制这台电脑——权限等同于有人直接坐在电脑前操作。",
      "s.accessTitle": "访问范围",
      "s.accessHint": "保存后，请在这台机器的终端执行 homebase restart。",
      "s.accessLocal": "仅本机",
      "s.accessLocalBody": "只监听 127.0.0.1，数据不会离开本机。默认且最安全。",
      "s.accessPrivate": "信任网段",
      "s.accessPrivateBody": "绑定下方信任网段内的地址（例如你的 Tailscale/WireGuard 地址）。",
      "s.accessLan": "局域网（有风险）",
      "s.accessLanBody": "监听 0.0.0.0。明文 HTTP，同网络下任何人都能访问。仅限完全信任的网络使用。",
      "s.rangesTitle": "信任网段",
      "s.rangesHint": "每行一个 CIDR 或 IP。这些地址是你自己声明的“已经被其他方式加密”的网络。示例：",
      "s.save": "保存",
      "s.saved": "已保存。请在这台机器的终端执行：homebase restart",
      "s.ackLan": "我明白这会把 Homebase 以明文 HTTP 暴露给我的局域网，同一网络下的任何人都可能借此完全控制这台电脑。",
      "s.ackRequired": "请先勾选上方确认框，确认你已了解风险。",
      "s.devicesTitle": "已配对设备",
      "s.devicesHint": "Access 不是「仅本机」时，这些浏览器可以打开 Homebase。吊销后，下一请求起该设备立即失效。",
      "s.devicesEmpty": "还没有配对设备。把 Access 改成非「仅本机」之后，在这台机器上执行 homebase pair。",
      "s.devicesRevoke": "吊销"
    }
  };

  const LANG_KEY = "homebase.lang";

  function detect() {
    try {
      const saved = localStorage.getItem(LANG_KEY);
      if (saved === "en" || saved === "zh") {
        return saved;
      }
    } catch (e) { /* private mode */ }
    return (navigator.language || "en").toLowerCase().indexOf("zh") === 0 ? "zh" : "en";
  }

  let lang = detect();

  function t(key, vars) {
    const table = DICT[lang] || DICT.en;
    let s = table[key] != null ? table[key] : (DICT.en[key] != null ? DICT.en[key] : key);
    if (vars) {
      Object.keys(vars).forEach(function (k) {
        s = s.replace("{" + k + "}", vars[k]);
      });
    }
    return s;
  }

  function apply(root) {
    root = root || document;
    root.querySelectorAll("[data-i18n]").forEach(function (el) {
      el.textContent = t(el.getAttribute("data-i18n"));
    });
    root.querySelectorAll("[data-i18n-title]").forEach(function (el) {
      el.title = t(el.getAttribute("data-i18n-title"));
    });
    root.querySelectorAll("[data-i18n-aria]").forEach(function (el) {
      el.setAttribute("aria-label", t(el.getAttribute("data-i18n-aria")));
    });
    root.querySelectorAll("[data-i18n-placeholder]").forEach(function (el) {
      el.placeholder = t(el.getAttribute("data-i18n-placeholder"));
    });
  }

  function setLang(l) {
    lang = (l === "zh") ? "zh" : "en";
    try { localStorage.setItem(LANG_KEY, lang); } catch (e) { /* private mode */ }
    document.documentElement.lang = lang === "zh" ? "zh-Hans" : "en";
    apply(document);
    document.dispatchEvent(new CustomEvent("homebase:lang-changed", { detail: { lang: lang } }));
  }

  window.homebaseI18n = { t: t, apply: apply, setLang: setLang, lang: function () { return lang; } };

  document.addEventListener("DOMContentLoaded", function () {
    document.documentElement.lang = lang === "zh" ? "zh-Hans" : "en";
    apply(document);
    const btn = document.getElementById("btn-lang");
    if (btn) {
      btn.textContent = lang === "zh" ? "中" : "EN";
      btn.addEventListener("click", function () {
        setLang(lang === "zh" ? "en" : "zh");
        btn.textContent = lang === "zh" ? "中" : "EN";
      });
    }
  });
})();
