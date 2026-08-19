# Homebase

自托管的 Web 终端会话入口。跑在家里常开的那台机器上（例如 Mac mini），浏览器经 Tailscale 打开后，自动 attach 到各设备上已有的 tmux session。关浏览器、笔记本休眠、网络抖一下，工作还在 tmux 里。

实现合同：[`AGENT.md`](AGENT.md)。技术方案：[`docs/DESIGN.md`](docs/DESIGN.md)（Accepted）。顺序：[`docs/PLAN.md`](docs/PLAN.md)。

文档已定稿。实现按 PLAN 做五个顺序 commit，不要揉成一个「先能看」的大提交。

## 它不是什么

- 不是多用户堡垒机，不是公网 shell 网关
- 不保存 SSH 密码
- 不在网页里自绘分屏或把 tmux window 映射成浏览器 Tab；右侧就是普通 tmux 客户端

## 运行前你需要

1. **Tailscale** 已在 Homebase 机器和你的客户端设备上登录同一 tailnet。
2. 从 **Homebase 这台机器** 能对每个目标 host **密钥登录**：

   ```bash
   ssh-copy-id user@host
   ssh user@host            # 必须成功一次，写入 known_hosts
   ```

   Homebase 使用 `BatchMode=yes`，不会在网页里帮你点 `yes/no` 或打密码。连本机也走 ssh，配 `user@localhost`。

3. 目标机器装了 **tmux**（Apple Silicon 上常见是 Homebrew，路径 `/opt/homebrew/bin/tmux`）。Homebase 会自己补 PATH；没装时 UI 会明确说，而不会假装会话丢了。

4. 不要把进程听在公网。默认：能探测到 Tailscale IPv4 就绑那个地址，否则 `127.0.0.1`。听 `0.0.0.0` 必须在配置里打开 `allow_public_bind`。

## 配置

默认文件：`~/.config/homebase/config.json`（可用 `-config` 改）。第一次启动若文件不存在，会写入一份空默认（`windows: []`，目录 0700，文件 0600）。形态见 AGENT.md。

可选 HTTP Basic Auth（默认关）。打开前用 `homebase hash-password` 生成 bcrypt，不要把明文密码写进 JSON。

## 启动

```bash
go build -o homebase ./cmd/homebase
./homebase
```

日志里会打印实际 bind 地址。用 Tailscale 那台设备的浏览器打开 `http://<tailscale-ip>:7681`。

## 安全摘要

- SSH：只用已有密钥，不存密码，不关 `StrictHostKeyChecking`
- 删 UI 里的 Window **不会** `tmux kill-session`
- 不要在日志或 issue 里贴密钥、bcrypt、终端输出
