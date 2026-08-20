# Homebase

*[English](README.md)*

自托管的 Web 终端。跑在家里常开的那台机器上（Mac mini、家里的小服务器）。
浏览器打开，就是这台机器上的 `tmux` session。

关标签、笔记本休眠、网络抖一下，工作还在 tmux 里。Homebase 只是桥。

UI 里的窗口列表就是这个 session 的 tmux window。新建、改名、关掉、切换，
都在网页上点。你不用给 session 起名，也不用按 `C-b`。

## 安装

宿主是 macOS（Apple Silicon 或 Intel）和 Linux。另外只需要 **tmux**。
Windows、iPad、手机当浏览器用，不当宿主。

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
homebase start
```

然后在这台机器上打开 **http://127.0.0.1:1990**。

如果只用这台电脑，到这里就结束了。不用配对，没有密码。

安装脚本把二进制放到 `~/.local/bin`。`homebase start` 会写成用户服务
（macOS 是 launchd，Linux 是 systemd），开机自起，命令立刻回到 prompt。

指定版本或回滚：在 `curl` 前面加 `HOMEBASE_VERSION=v0.1.0`。升级就是再跑一遍
安装脚本；如果服务已经在跑，接着执行 `homebase restart`。

## 给别的设备用（手机、笔记本）

没有密码。给一台设备授权的意思是：你已经能在这台机器上执行命令，所以可以
签发一个一次性链接，拿到那台设备上打开。

1. 在这台机器上打开 [http://127.0.0.1:1990/settings.html](http://127.0.0.1:1990/settings.html)
2. 把 **Access** 设成 **信任网段**（你的 Tailscale / WireGuard），或者你清楚
   风险的话设成 **局域网**
3. 保存，然后在这台机器的终端里：

```bash
homebase restart
homebase pair
```

4. 把打印出来的 URL 在另一台设备上打开。之后这台设备一直免登录。

每多一台设备就再跑一次 `homebase pair`。吊销在 Settings 同一页（只能从这台
机器打开）。

## 命令

| 命令 | 作用 |
| --- | --- |
| `homebase start` | 写成用户服务并拉起 |
| `homebase stop` | 停服务。配置和 tmux 都不动 |
| `homebase restart` | 先 stop 再 start。改完 Access 用这一条 |
| `homebase status` | 在不在跑、绑定地址、URL、tmux、版本 |
| `homebase pair` | 一次性登录链接，10 分钟有效 |
| `homebase version` | 构建版本 |

## ⚠️ 暴露到本机以外之前，先读这一段

Homebase 是**明文 HTTP，没有 HTTPS**。能打开这个网页的人，就能以运行
Homebase 的那个用户的身份，在这台机器上执行命令——和坐在键盘前一样。

安不安全，完全取决于绑在哪：

| 绑在哪 | 安全吗 |
| --- | --- |
| `127.0.0.1`（默认） | 安全。流量不会离开这台机器。 |
| 你在 Settings 里配的**信任网段**（Tailscale、WireGuard 等） | 只有当那一段确实已经被加密时才安全。Homebase 无法验证——这是你自己的声明。 |
| 其他任何网络（局域网、酒店、办公室 Wi-Fi） | **不安全。** 同一网络上的人可以截获会话、拿走这台机器。 |

`access` 是唯一的旋钮，在 Settings 里改，不在 CLI 里：

| Access | 绑定 | 需要配对 |
| --- | --- | --- |
| 仅本机（默认） | `127.0.0.1` | 否 |
| 信任网段 | 信任网段里的第一个本机地址，找不到则回落到 loopback | 是 |
| 局域网 | `0.0.0.0` | 是 |

「仅本机」免登录，是因为能连到 `127.0.0.1` 就说明已经能在这台机器上开终端了。
（这一档会钉死 `Host` 头，防止网页用 DNS rebinding 打进来。）

除此之外都要配对——但配对是身份验证，不是加密。它能挡住路过的人打开网页，
**挡不住**同一个不受信网络上的人从流量里读出 session cookie。只把 Homebase
放在你完全掌控的网络上。

## Settings 和配置

Settings（`/settings.html`）即使 Access 已经是信任网段或局域网，也只能从
`127.0.0.1` 打开。改 Access、改信任网段、吊销设备，永远要求人在这台机器上。

保存 Access 之后，在这台机器的终端执行 `homebase restart`。网页不会自己把
进程拉起来。

默认配置：`~/.config/homebase/config.json`（`start` / `serve` / `status` /
`pair` 可用 `-config` 覆盖）。第一次运行会写一份安全默认值：`access: local`，
目录 `0700`，文件 `0600`。

```json
{
  "version": 4,
  "access": "local",
  "listen_addr": "",
  "listen_port": 1990,
  "trusted_ranges": ["100.64.0.0/10"]
}
```

`trusted_ranges` 是你声明「已经被别的东西加密了」的 CIDR 列表。默认是
Tailscale 的 CGNAT 段。用别的组网就换成自己的。它不是密码学边界，只决定
`private` 档绑哪个地址，以及 Settings 显示哪条警告。

## 从源码构建

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```

`start` 登记的是你刚刚跑的那个二进制，不会拷到 `~/.local/bin`。
重新 vendor xterm.js（几乎用不到）：`./scripts/vendor-xterm.sh`。

前台跑、不装服务（排错）：`./homebase serve`。

## 安全摘要

- 永远是明文 HTTP。如果有加密，来自你把 Homebase 放进去的那个网络，不是
  Homebase 自己。
- 没有密码。配对要求「已经能在这台机器上执行命令」，所以不会带来新权限。
- 配对 token 10 分钟过期、一次性；落盘的只有 SHA-256 哈希。
- 离开 loopback 就必须配对设备。`trusted_ranges` 是 Settings 警告用的标签，
  不是身份证明。
- Settings 和吊销设备只能从 loopback 访问。
- 永不 `kill-server` / `kill-session`；session 里最后一个 window 拒绝关闭。
- 不要在日志或 issue 里贴密钥、密码哈希或原始终端输出。
