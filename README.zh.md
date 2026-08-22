# Homebase

*[English](README.md)*

## 这是什么

如果你有一台 24 小时不关机的机器（Mac mini 或 MacBook），习惯在终端（TUI）里长时间运行 Claude Code、Grok Build 一类的命令行 agent 工具，Homebase 让你可以从手机或另一台电脑，通过浏览器方便地接管这个终端会话——不需要额外的 SSH 客户端，也不必让这台机器暴露在公网上。

Homebase 是一个 Go 编写的单文件程序，运行在这台常开机器上。它把浏览器连接到该机器上**一个固定的 tmux session**（名字固定为 `homebase`），侧边栏显示的是这个 session 里的 tmux 窗口列表。所有会话状态都由 tmux 持有，Homebase 本身只是一条 PTY 转发通道：关闭标签页、笔记本合盖、网络抖动，都不会中断正在运行的程序。

![Homebase 网页终端截图](.github/images/homebase-ui.png)

## 前置依赖

**tmux（必须）。** Homebase 不管理会话状态，真正让"合盖不断线、换设备接着用"成立的是 tmux——浏览器只是接上了一个持续运行的 tmux session，断开重连时，tmux 用它自己的 scrollback 把画面重绘出来。没有 tmux，Homebase 无法工作。

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

**Tailscale（强烈建议）。** Homebase 自身以明文 HTTP 提供服务，不做公网身份认证意义上的强加密传输。它依赖 Tailscale 在你的设备之间组建一个私有网络，让"从手机连回家里的机器"这件事不需要把任何端口暴露到公网上。Homebase 的监听地址策略也是围绕这一点设计的：默认只绑定回环地址和 Tailscale 网段，从不监听公网或未指定地址。宿主机和要连进来的设备都需要装好 Tailscale 并登录同一个 tailnet。

## 安装与启动

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
```

这条命令首次运行是安装，之后再次运行就是升级——它会重新下载最新版本二进制，原子替换掉 `~/.local/bin/homebase`。如果服务已经在跑，升级后需要手动执行一次 `homebase restart` 让新版本生效。

安装完成后启动服务：

```bash
homebase start
```

在**这台机器本机**打开命令行提示的地址（默认 [http://127.0.0.1:1990](http://127.0.0.1:1990)）。默认情况下只有 `127.0.0.1` 和这台机器的 Tailscale 地址可以访问，局域网地址（如 `192.168.x.x`）默认不可达。如果希望局域网内的其他设备也能直接访问，在 Settings 的「访问范围」里选择「所有局域网」，保存后服务会自动重启；也可以用命令行完成同样的操作：`homebase access lan && homebase restart`。

若要从另一台设备（手机、另一台电脑）连接：

1. 在宿主机上打开 Settings（仅在访问 `127.0.0.1` 时可见），把 Access 改成信任网段，保存后服务会自动重启。
2. 执行 `homebase pair`，把打印出的一次性链接发到另一台已经加入同一 tailnet 的设备上打开。首次配对必须先用普通方式（本机终端或 SSH）登录到这台机器再执行这条命令——能在这台机器上跑命令，本身就是在证明你有权限。

`homebase pair` 的输出大致如下：

```
Open this link on the device you want to pair:

    http://100.101.102.103:1990/pair?t=9f2a6c1e4b7d8a3f5c0e1b2d3a4f5c6e

Valid until 21:45:10 (10 minutes), single use only.
```

在另一台设备的浏览器里打开这个链接，就会换取一个长期有效的登录 cookie。

### macOS：建议提前开启完整磁盘访问权限

如果宿主机是 macOS，建议现在就把 `~/.local/bin/homebase` 加入 系统设置 → 隐私与安全性 → 完全磁盘访问权限。macOS 对文稿、桌面、下载等目录的访问需要用户在图形界面里手动点击授权；如果没有提前设置，等你人在异地、只能通过手机或远程终端操作，而这台机器又没有开着屏幕共享时，一旦 Homebase 需要读取这些目录就会被系统静默拦下，而你没有办法远程点掉那个确认框。

## 命令

| 命令 | 作用 |
| --- | --- |
| `homebase start` | 注册为用户级后台服务（macOS 是 launchd agent，Linux 是 systemd user unit）并启动 |
| `homebase stop` | 停止服务。配置文件和 tmux session 都不受影响 |
| `homebase restart` | 先 stop 再 start，用于升级二进制后让新版本生效 |
| `homebase status` | 打印是否在运行、tmux 路径、版本号、bind 地址、URL、是否需要配对、已启用设备数等 |
| `homebase pair` | 生成一条一次性登录链接，10 分钟内有效，只能使用一次 |
| `homebase version` | 打印当前二进制的构建版本 |

不带参数运行 `homebase` 只打印帮助信息，不会启动服务。此外还有一个 `homebase serve`，它是 `start` 内部注册给系统服务管理器的前台进程，平时不需要手动调用，仅用于调试。

一台机器只需要跑一份 Homebase。重复执行 `start` 不会启动第二个实例，而是打印当前已在使用的地址。端口 1990 被占用时会依次尝试 1991、1992……并记住实际使用的端口。

## 安全模型

Homebase 只有一个安全相关的配置项：Access（私有 / 局域网）。这个选项直接决定了服务监听的地址，而认证方式（是否需要配对）和是否使用 TLS，都是根据这个绑定地址**派生**出来的，不能单独配置——也就是说，不存在"监听公网地址但不需要凭证、明文传输"这种组合。Homebase 从设计上就拒绝绑定任何公网可路由地址或 `0.0.0.0`，`-listen` 参数也无法覆盖这一限制。这意味着即便配置有误，也不会不小心把一个没有保护的端口暴露到公网上。

回环地址（`127.0.0.1`）无需配对即可访问；其他设备则必须先完成配对（见上文「安装与启动」）。配对不会授予任何额外权限——执行 `homebase pair` 本身就要求先能在这台机器上跑命令，而这正是 Homebase 所交出的能力。

设备的会话凭证只以哈希形式保存在本机的 `devices.json` 中，明文的一次性令牌和会话密钥只出现一次——分别是 CLI 的标准输出和浏览器的 cookie。撤销某台设备的访问权限，可以在 Settings 页面（仅回环连接可见）完成。

## 从源码构建

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```
