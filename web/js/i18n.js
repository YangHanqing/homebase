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
      "aria.keyCtrl": "Ctrl (tap, then the key)",
      "aria.keyEsc": "Esc",
      "aria.keyTab": "Tab",
      "aria.keyUp": "Up",
      "aria.keyDown": "Down",
      "aria.keyLeft": "Left",
      "aria.keyRight": "Right",
      "aria.keyIntr": "Ctrl+C (interrupt)",
      "aria.keyMore": "All keys",
      "keys.title": "All keys",
      "keys.modTitle": "Modifiers",
      "keys.modHint": "Tap one, then type the key it should combine with. It applies to the next character only.",
      "keys.ctrlTitle": "Control",
      "keys.navTitle": "Navigation",
      "keys.fnTitle": "Function",
      "keys.symTitle": "Symbols",
      "keys.dblTap": "Tip: double-tap the terminal to send Tab. Turn it off in Settings.",
      "keys.cA": "line start",
      "keys.cE": "line end",
      "keys.cW": "kill word",
      "keys.cU": "kill to start",
      "keys.cK": "kill to end",
      "keys.cC": "interrupt",
      "keys.cD": "end of input",
      "keys.cZ": "suspend",
      "keys.cL": "clear",
      "keys.cR": "history",
      "common.close": "Close",
      "sidebar.windows": "Windows",
      "sidebar.new": "+ New",
      "sidebar.projects": "Projects",
      "sidebar.addProject": "+ Project",
      "sidebar.ungrouped": "Ungrouped",
      "sidebar.collapse": "Collapse",
      "sidebar.expand": "Expand",
      "project.newWindowAria": "New window in {name}",
      "project.delete": "Delete project",
      "project.deleteAria": "Delete project {name}",
      "project.deleteConfirm": "Delete project \"{name}\"? This also ends its terminal session; anything still running in it is stopped.",
      "project.add.title": "Add project",
      "project.add.hint": "A project is a folder. New windows created under it start there.",
      "project.add.path": "Folder",
      "project.add.browse": "Browse…",
      "project.add.err.required": "Choose a folder first.",
      "browse.title": "Choose a folder",
      "browse.select": "Select this folder",
      "browse.up": "↑ Up",
      "browse.empty": "No subfolders here.",
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
      "window.selectAria": "Window {index}: {name} — {state}",
      "window.act.busy": "Working now",
      "window.act.recent": "Active in the last few minutes",
      "window.act.quiet": "Quiet",
      "window.act.bell": "Rang the bell — needs a look",
      "list.empty": "No windows yet — one appears as soon as you connect.",
      "list.unpaired": "This device is no longer paired. Reloading…",
      "list.serverError": "Homebase is not answering. Retrying.",
      "status.connected": "Connected",
      "status.connecting": "Connecting",
      "status.disconnected": "Disconnected",
      "status.disconnectedHint": "Disconnected · retrying",
      "status.error": "Error",
      "sidebar.clients": "· {count} live",
      "sidebar.clientsHint": "· {count} watching",
      "toast.copied": "Copied",
      "help.enotmux": "tmux is not installed on this machine. Install it and reconnect to start a new session.",
      "help.pty_spawn": "Could not start the local tmux session. Check the Homebase process log.",
      "help.ws_closed": "",

      "theme.system": "Sys",
      "theme.dark": "Dark",
      "theme.light": "Light",
      "theme.title": "Theme: {name}",

      "s.back": "← Back",
      "s.title": "Settings",
      "s.tabAppearance": "Preferences",
      "s.tabSecurity": "Security",
      "s.prefsScope": "Stored in this browser only. Each device keeps its own settings, and these never change how Homebase listens on the network.",
      "s.langTitle": "Language",
      "s.langAuto": "Auto",
      "s.langAutoBody": "Match this browser's language.",
      "s.behaviourTitle": "Behaviour",
      "s.copySelect": "Copy selected text automatically",
      "s.copySelectBody": "Selecting text in the terminal puts it on this device's clipboard. The selection never goes to the terminal or the host.",
      "s.cursorBlink": "Blinking cursor",
      "s.cursorBlinkBody": "Turn off to save a little battery on a screen left open.",
      "s.doubleTapTab": "Double-tap sends Tab",
      "s.doubleTapTabBody": "On a touch screen only. Double-tapping the terminal sends Tab instead of selecting a word.",
      "s.terminalTitle": "Terminal",
      "s.newWindowDir": "New window directory",
      "s.newWindowDirSame": "Same as current window",
      "s.newWindowDirHome": "Home (~)",
      "s.fontSize": "Font size",
      "s.fontSizeBody": "Changing this re-measures the grid and tells tmux the new size.",
      "s.scrollback": "Scrollback",
      "s.scrollbackBody": "Lines kept in the browser. tmux keeps its own history separately.",
      "s.loopbackOnly": "Settings can only be viewed and changed from a browser running on this machine (127.0.0.1). Open this page locally to make changes.",
      "s.accessTitle": "Access",
      "s.accessHint": "Saving restarts the service automatically.",
      "s.accessPrivate": "Trusted range",
      "s.accessPrivateBody": "Default. This machine on 127.0.0.1, plus addresses in the ranges below.",
      "s.accessLan": "All local networks",
      "s.accessLanBody": "Every private IPv4 on this machine. Public addresses are never bound.",
      "s.rangesTitle": "Trusted ranges",
      "s.rangesHint": "One private CIDR or IP per line, at most 5. Overlay networks you assert are already encrypted (Tailscale, WireGuard, …).",
      "s.rangesExample": "Example: 100.64.0.0/10 (Tailscale), 192.168.0.0/16 (LAN)",
      "s.rangesMax": "At most 5 ranges.",
      "s.save": "Save",
      "s.saved": "Saved.",
      "s.confirmBody": "Homebase uses unencrypted HTTP. When deployed on a regular LAN, other devices on the same network may perform a man-in-the-middle attack; security depends on whether that network is trusted. Prefer access via an already-encrypted network such as Tailscale or WireGuard.",
      "s.confirmSave": "Save",
      "s.restarting": "Restarting…",
      "s.restartTimeout": "Saved, but the service did not come back. On this machine, run:  homebase restart",
      "s.devicesTitle": "Paired devices",
      "s.devicesHint": "Browsers that can reach Homebase from another device. Revoking logs that device out on the next request.",
      "s.devicesEmpty": "No devices paired yet. Run homebase pair on this machine.",
      "s.devicesRevoke": "Revoke",
      "s.devicesRevokeAll": "Revoke all",
      "s.devicesRevokeAllBody": "Revoke every paired device? They will need to run homebase pair again."
    },
    zh: {
      "aria.openWindows": "打开窗口列表",
      "aria.closeWindows": "关闭窗口列表",
      "aria.keyCtrl": "Ctrl（先点这个，再点下一个键）",
      "aria.keyEsc": "Esc",
      "aria.keyTab": "Tab",
      "aria.keyUp": "上",
      "aria.keyDown": "下",
      "aria.keyLeft": "左",
      "aria.keyRight": "右",
      "aria.keyIntr": "Ctrl+C（中断）",
      "aria.keyMore": "全部按键",
      "keys.title": "全部按键",
      "keys.modTitle": "修饰键",
      "keys.modHint": "先点一个，再输入要组合的那个键。只对下一个字符生效。",
      "keys.ctrlTitle": "控制组合",
      "keys.navTitle": "导航",
      "keys.fnTitle": "功能键",
      "keys.symTitle": "符号",
      "keys.dblTap": "提示：在终端上双击即可发送 Tab。可在设置中关闭。",
      "keys.cA": "行首",
      "keys.cE": "行尾",
      "keys.cW": "删一词",
      "keys.cU": "删到行首",
      "keys.cK": "删到行尾",
      "keys.cC": "中断",
      "keys.cD": "结束输入",
      "keys.cZ": "挂起",
      "keys.cL": "清屏",
      "keys.cR": "搜历史",
      "common.close": "关闭",
      "sidebar.windows": "窗口",
      "sidebar.new": "＋ 新建",
      "sidebar.projects": "项目",
      "sidebar.addProject": "＋ 项目",
      "sidebar.ungrouped": "未分组",
      "sidebar.collapse": "折叠",
      "sidebar.expand": "展开",
      "project.newWindowAria": "在 {name} 中新建窗口",
      "project.delete": "删除项目",
      "project.deleteAria": "删除项目 {name}",
      "project.deleteConfirm": "删除项目“{name}”？这会连它的终端 session 一起关闭，里面还在跑的东西也会被终止。",
      "project.add.title": "新建项目",
      "project.add.hint": "项目就是一个文件夹，在它下面新建的窗口都从这个目录开始。",
      "project.add.path": "文件夹",
      "project.add.browse": "浏览…",
      "project.add.err.required": "请先选择一个文件夹。",
      "browse.title": "选择文件夹",
      "browse.select": "选择此文件夹",
      "browse.up": "↑ 上一级",
      "browse.empty": "这里没有子文件夹。",
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
      "window.selectAria": "窗口 {index}：{name} —— {state}",
      "window.act.busy": "正在输出",
      "window.act.recent": "几分钟内有过输出",
      "window.act.quiet": "安静",
      "window.act.bell": "响过提示音，去看一眼",
      "list.empty": "还没有窗口，连上就会有一个。",
      "list.unpaired": "此设备已不再配对，正在重新加载…",
      "list.serverError": "Homebase 没有响应，重试中。",
      "status.connected": "已连接",
      "status.connecting": "连接中",
      "status.disconnected": "已断开 · 重试中",
      "status.disconnectedHint": "已断开 · 重试中",
      "status.error": "出错",
      "sidebar.clients": "· {count} 人在看",
      "sidebar.clientsHint": "· {count} 人在看",
      "toast.copied": "已复制",
      "help.enotmux": "这台机器上没装 tmux。装好后重连即可开始新 session。",
      "help.pty_spawn": "本机 tmux/pty 没拉起来。请查看 Homebase 进程日志。",
      "help.ws_closed": "",

      "theme.system": "系统",
      "theme.dark": "深色",
      "theme.light": "浅色",
      "theme.title": "主题：{name}",

      "s.back": "← 返回",
      "s.title": "设置",
      "s.tabAppearance": "偏好",
      "s.tabSecurity": "安全",
      "s.prefsScope": "只保存在当前浏览器中。每台设备各自独立，且不会影响 Homebase 在网络上的监听方式。",
      "s.langTitle": "语言",
      "s.langAuto": "自动",
      "s.langAutoBody": "使用本浏览器的语言设置。",
      "s.behaviourTitle": "行为",
      "s.copySelect": "选中文本自动复制",
      "s.copySelectBody": "在终端中选中文本即复制到本设备剪贴板。选中内容不会被发送到终端或主机。",
      "s.cursorBlink": "光标闪烁",
      "s.cursorBlinkBody": "长时间开着页面时，关掉可以省一点电。",
      "s.doubleTapTab": "双击发送 Tab",
      "s.doubleTapTabBody": "仅触摸屏。在终端上双击发送 Tab，而不是选中一个词。",
      "s.terminalTitle": "终端",
      "s.newWindowDir": "新建窗口的目录",
      "s.newWindowDirSame": "和当前窗口相同",
      "s.newWindowDirHome": "主目录（~）",
      "s.fontSize": "字号",
      "s.fontSizeBody": "修改后会重新计算终端网格，并把新尺寸告知 tmux。",
      "s.scrollback": "回滚行数",
      "s.scrollbackBody": "浏览器中保留的行数。tmux 自己另有一份历史记录。",
      "s.loopbackOnly": "设置只能在本机（127.0.0.1）打开的浏览器中查看和修改。请在本机打开这个页面进行修改。",
      "s.accessTitle": "访问范围",
      "s.accessHint": "保存后服务会自动重启。",
      "s.accessPrivate": "信任网段",
      "s.accessPrivateBody": "默认。本机 127.0.0.1，加上下方信任网段内的地址。",
      "s.accessLan": "所有局域网",
      "s.accessLanBody": "本机上每一个私网 IPv4。公网地址不会监听。",
      "s.rangesTitle": "信任网段",
      "s.rangesHint": "每行一个私网 CIDR 或 IP，最多 5 行。这些是你声明已经被加密的 overlay 网络（Tailscale、WireGuard 等）。",
      "s.rangesExample": "示例：100.64.0.0/10（Tailscale）、192.168.0.0/16（局域网）",
      "s.rangesMax": "最多 5 个网段。",
      "s.save": "保存",
      "s.saved": "已保存。",
      "s.confirmBody": "Homebase 使用未加密的 HTTP。部署于普通局域网时，存在同网中间人攻击的可能，安全性取决于该网络是否可信。建议经由 Tailscale、WireGuard 等已加密网络访问。",
      "s.confirmSave": "确认保存",
      "s.restarting": "正在重启…",
      "s.restartTimeout": "已保存，但服务没有恢复。请在本机执行：homebase restart",
      "s.devicesTitle": "已配对设备",
      "s.devicesHint": "可以从其他设备打开 Homebase 的浏览器。吊销后，下一请求起该设备立即失效。",
      "s.devicesEmpty": "还没有配对设备。在这台机器上执行 homebase pair。",
      "s.devicesRevoke": "吊销",
      "s.devicesRevokeAll": "吊销全部",
      "s.devicesRevokeAllBody": "吊销所有已配对设备？它们需要重新执行 homebase pair。"
    }
  };

  const LANG_KEY = "homebase.lang";

  function browserLang() {
    return (navigator.language || "en").toLowerCase().indexOf("zh") === 0 ? "zh" : "en";
  }

  function readPref() {
    try {
      const saved = localStorage.getItem(LANG_KEY);
      if (saved === "en" || saved === "zh" || saved === "auto") {
        return saved;
      }
    } catch (e) { /* private mode */ }
    return "auto";
  }

  function resolve(p) {
    return (p === "zh" || p === "en") ? p : browserLang();
  }

  function htmlLang(code) {
    return code === "zh" ? "zh-Hans" : "en";
  }

  let pref = readPref();
  let lang = resolve(pref);

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
      // textContent wipes descendants. Skip anything that still has a control
      // nested in it — that is how the rename field used to disappear on load.
      if (el.querySelector("input, textarea, select, button")) {
        return;
      }
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

  function commit() {
    document.documentElement.lang = htmlLang(lang);
    apply(document);
    document.dispatchEvent(new CustomEvent("homebase:lang-changed", {
      detail: { lang: lang, pref: pref }
    }));
  }

  function setLang(l) {
    pref = (l === "zh" || l === "en" || l === "auto") ? l : "auto";
    lang = resolve(pref);
    try { localStorage.setItem(LANG_KEY, pref); } catch (e) { /* private mode */ }
    commit();
  }

  window.addEventListener("languagechange", function () {
    if (pref !== "auto") {
      return;
    }
    const next = resolve(pref);
    if (next === lang) {
      return;
    }
    lang = next;
    commit();
  });

  window.homebaseI18n = {
    t: t,
    apply: apply,
    setLang: setLang,
    lang: function () { return lang; },
    pref: function () { return pref; }
  };

  document.addEventListener("DOMContentLoaded", function () {
    document.documentElement.lang = htmlLang(lang);
    apply(document);
  });
})();
