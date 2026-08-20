# Homebase

*[English](README.md)*

给家里常开的那台机器用的 Web 终端。浏览器打开，就是这台机器上的 `tmux`
session。关标签、休眠、网络抖一下，工作还在。

Homebase 不替代 tmux。它只挂上**一个** session，名字固定叫 `homebase`。

**安全性靠 Tailscale，不靠 Homebase。** Homebase 是明文 HTTP。别的设备请走
Tailscale。不要把它直接放到裸局域网上，除非你清楚风险。

## 你需要什么

**宿主**（跑 `homebase` 的那台机器）：

| 系统 | CPU |
| --- | --- |
| macOS | Apple Silicon 或 Intel |
| Linux | x86_64 或 ARM |

Windows 不能当宿主。Windows、iPad、手机只当浏览器。

另外：

1. 宿主上装着 **tmux**（必须）
2. 宿主和要连进来的设备都装着 **Tailscale**

## 1. 安装 tmux

```bash
# macOS（Homebrew）
brew install tmux

# Debian / Ubuntu
sudo apt install tmux

# Fedora
sudo dnf install tmux

# Arch
sudo pacman -S tmux
```

## 2. 安装 Homebase

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
```

这一步只把二进制放到 `~/.local/bin`，不会启动服务。

## 3. 启动

```bash
homebase start
```

在**这台机器**上打开 [http://127.0.0.1:1990](http://127.0.0.1:1990)。

默认可以访问的是 `127.0.0.1` 和你的 Tailscale 地址，**不是** `192.168.x.x`。

## 给别的设备用（Tailscale）

1. 在宿主上打开 Settings（齿轮；只在 `127.0.0.1` 显示）
2. Access 选 **信任网段**。保存，然后：

```bash
homebase restart
homebase pair
```

3. 另一台设备连着 Tailscale，打开打印出来的 URL。

## 更新

再跑一遍安装脚本，然后重启服务：

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
homebase restart
```

指定版本：在 `curl` 前面加 `HOMEBASE_VERSION=v0.1.1`。

## 不用 Tailscale：走局域网

这**不是**主路径。Settings → Access → **局域网（有风险）**，保存时会要求确认，
然后 `homebase restart` 和 `homebase pair`。不要在酒店或办公室 Wi-Fi 上这么做。

## 命令

| 命令 | 作用 |
| --- | --- |
| `homebase start` | 写成用户服务并拉起 |
| `homebase stop` | 停服务。配置和 tmux 都不动 |
| `homebase restart` | 先停再起。改完 Access 用这一条 |
| `homebase status` | 在不在跑、URL、tmux、版本 |
| `homebase pair` | 一次性登录链接（10 分钟，只能用一次） |
| `homebase version` | 构建版本 |

一台机器只跑一份。已经在跑再 `start` 会打印当前 URL，不会再起一个。
1990 被别的程序占用时，会试 1991、1992… 并记下用上的端口。

## 从源码构建

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```
