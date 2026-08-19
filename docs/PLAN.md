# Homebase 实现计划

按 PR 顺序做。每一个 PR 都要能单独 review、能 `go test ./...` 通过、不留下半截协议。细节与禁令以 [`DESIGN.md`](DESIGN.md) 和 [`../AGENT.md`](../AGENT.md) 为准。文档已 Accepted。

绿场仓库：这些「PR」是同一 repo 上的顺序 commit。不要把 PR1–PR4 揉成一个「先能看」的大爆炸。

---

## PR1 — Scaffolding, config, listen policy

**Title:** `scaffold: config store, listen-address policy, health endpoint`

**Files:**

- `go.mod`
- `cmd/homebase/main.go`
- `internal/config/*`
- `internal/listen/*`
- `internal/ident/*`
- `internal/api/health.go`（或放在 `internal/server` 的空壳 mux）
- `web/index.html` 占位一页（证明 embed 通）
- `*_test.go`：ident、config 原子写、缺文件写默认、listen 拒绝 `0.0.0.0`

**Depends on:** none

**Does:**

- Flag：`-config`、`-listen`、`-port`、`-log-level`
- 默认配置路径、0700/0600、原子写；文件不存在则写入空默认并继续
- Tailscale IPv4 探测 + loopback 回退；`allow_public_bind` 门闩（`-listen` 不能绕过）
- `GET /api/health`
- embed 一份 hello HTML
- 还 **没有** ssh、没有 WS、没有 xterm

**Done when:** 在没有 Tailscale 的机器上启动绑到 `127.0.0.1:7681`；`-listen 0.0.0.0:7681` 且 `allow_public_bind` 未开时进程拒绝启动；`ident` 测试拒绝 `host='; rm'`；缺配置文件时落盘一份 `windows: []`。

---

## PR2 — Windows REST API

**Title:** `api: CRUD /api/windows against JSON store`

**Files:**

- `internal/api/windows.go`
- `internal/config` 增删改、reorder、32 顶
- Origin 校验
- 测试：CRUD 内存/临时文件、校验失败 400、DELETE 只碰 JSON

**Depends on:** PR1

**Does:** 完整 REST。服务端发 UUID。无 PTY。`PUT /api/windows/reorder` 一并做。

**Done when:** curl 增删改后文件内容正确；非法 `session_name` 进不了盘。

---

## PR3 — SSH PTY + WebSocket protocol

**Title:** `session: ssh pty bridge and websocket framing`

**Files:**

- `internal/session/*`（`Dialer`、`SSHDialer`、`Proc`、进程组 Kill）
- `internal/ws/*`
- 固定 ssh argv 与远端 `/bin/sh -c` 脚本（含 `HOMEBASE_ENOTMUX`）
- 测试：用 local `tmux` 的测试 Dialer 覆盖 Start / Write / Read / SetSize / Kill；协议测试（resize JSON、binary、未知 type、未知 id → 4404）
- 可选：`cmd/homebase` 临时用一个最小 WS 页面或 `websocat` 文档做手工探针

**Depends on:** PR2（要有 window id）

**Does:**

- Model A：WS 开则 spawn，关则 Kill ssh pgid
- binary 数据 + JSON 控制
- `pty.Setsize` 必须在这条 PR 可测，不是 PR4 才做
- stderr → `code` 映射（auth / hostkey / timeout / enotmux）
- **不**杀远端 session
- **不**做连接数限额

**Done when:**

- 单测不需要第二台机器
- 手工：`websocat` 连 `/ws/windows/{id}` 能看到 tmux 画面字节、发 resize JSON 后远端 `stty size` 变
- 关 WS 后 `ps` 里没有残留 ssh；远端 `tmux ls` 仍在

---

## PR4 — Frontend: list, xterm, auto-connect, reconnect

**Title:** `web: window list, xterm per window, autostart, backoff`

**Files:**

- `scripts/vendor-xterm.sh`（钉 xterm.js 5.5.0）+ `web/vendor/xterm/*`
- `web/index.html`、`web/css/app.css`、`web/js/{app,session,terminal}.js`
- UI 状态点与错误文案（含 enotmux / hostkey 固定句）
- FitAddon + ResizeObserver；隐藏终端不用 `display:none`
- 系统字体，不 vendor `.woff`
- backoff 纯函数测试（抽到 `session.js` 可测处）

**Depends on:** PR3

**Does:**

- 打开页面自动连 **全部** Window
- 空列表显示空态 + 添加入口
- 切换列表不拆 WS
- 拖浏览器窗口，TUI 不错位
- 断网/关 WS 后 1s→2s→…→30s 重连，`term.reset()`
- CRUD 立刻开/关对应连接

**Done when:** 用两个 Window 配置（可都是 `user@localhost` 的不同 session 名）手工走完：刷新即双连、切来切去、缩放 vim、杀 WS 看重连、DELETE 后远端 session 还在。

视觉按 DESIGN.md §11，不要改成通用 dashboard。

---

## PR5 — Optional Basic Auth, operator docs, launchd example

**Title:** `auth: optional basic auth, README, hash-password`

**Files:**

- `internal/auth/*`
- `cmd/homebase` 子命令 `hash-password`
- `README.md` 补全：Tailscale、`ssh-copy-id`、先手动 `ssh` 写 known_hosts、远端 tmux、`allow_public_bind`、launchd 示例
- auth 开/关测试

**Depends on:** PR4

**Does:** 默认关。打开后静态 + API + WS 都要认证。浏览器同源 WS 带 Basic。

**Done when:** 未开启时行为与 PR4 相同；开启后无凭证 401；README 按空白 Mac mini 能跟做。

---

## 不做进这些 PR 的东西

- localhost 免 ssh
- 拖拽排序（reorder API 在 PR2，UI 拖拽以后再说）
- IPv6 Tailscale
- WS 连接数限额 / 4403
- 自托管字体
- CI 矩阵、Docker、发布签名（有空再加，不挡 V1）

---

## 建议的本地验收脚本（PR4 之后，手工）

在 Homebase 机器上：

```bash
ssh-copy-id "$USER@localhost"   # 若尚未做
tmux new-session -d -s work     # 预置一个，证明 attach 而非误杀
go run ./cmd/homebase
# 浏览器打开打印出的 URL
# 再：删除 UI 里的 Window，tmux ls 仍有 work
```

远端至少一台其它 host 再走一遍 `enotmux`（临时把 PATH 里的 tmux 改名）和恢复后 attach。
