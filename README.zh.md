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

**Tailscale（强烈建议）。** Homebase 自身以明文 HTTP 提供服务，没有 TLS，也没有任何开关能打开 TLS。它依赖 Tailscale 在你的设备之间组建一个私有网络，让"从手机连回家里的机器"这件事不需要把任何端口暴露到公网上。Homebase 的监听地址策略也是围绕这一点设计的：默认只绑定回环地址和 Tailscale 网段，从不监听公网或未指定地址。宿主机和要连进来的设备都需要装好 Tailscale 并登录同一个 tailnet。

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

如果宿主机是 macOS，现在就把 **Homebase 这个可执行文件** 加到 系统设置 → 隐私与安全性 → 完全磁盘访问权限。默认安装位置是 `~/.local/bin/homebase`，展开后是 `/Users/<你的用户名>/.local/bin/homebase`。`install.sh` 结束时会打印绝对路径；如果你设置过 `HOMEBASE_INSTALL_DIR`，用那个目录里的文件。

macOS **不会**为此弹出「允许」对话框，程序也无法自己申请这个权限。文稿、桌面、下载这类目录必须在系统设置里手动点一次。如果现在不设，等你人在异地、只能用手机或远程终端、这台机器又没开屏幕共享时，一旦 Homebase 要读这些目录，系统会静默拦住，你没有办法远程点「允许」。

`~/.local` 是隐藏目录，Finder 普通浏览看不到。打开「完全磁盘访问权限」之后：

1. 点 **+**。
2. Finder 菜单 **前往 → 前往文件夹…**（快捷键 ⇧⌘G）。
3. 粘贴 `~/.local/bin`，回车。
4. 选中名为 `homebase` 的**文件**（不要选文件夹），确认。

也可以把该文件从 Finder 窗口拖进完全磁盘访问列表。只授权文件夹、或授权了另一份拷贝，都不算。升级一般仍是同一路径，通常不用重新点。

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

Homebase 交给浏览器的是一个 shell。把它接到任何网络之前，请先读完这一节。

**Homebase 以明文 HTTP 提供服务，没有 TLS，也没有任何配置项能打开 TLS。** 传输加密是 Tailscale（或 WireGuard，或任何你已经信任的 overlay 网络）的职责——这也正是默认档位只绑定回环地址和 Tailscale 网段的原因。在普通局域网里，同网段的任何人都能看到你敲的每一个字符和终端打印的全部内容，也能直接从线路上截走会话 cookie 或配对链接。Settings → Access → 「所有本地网络」那段红色确认文案说的就是这件事。

Access（私有 / 局域网）是唯一一个安全相关的配置项。它决定服务监听的地址，而认证方式（是否需要配对）是根据这个绑定地址**派生**出来的，不能单独配置——也就是说，不存在「监听可路由地址但不需要凭证」这种组合。Homebase 从设计上就拒绝绑定任何公网可路由地址或 `0.0.0.0`，`-listen` 参数也无法覆盖这一限制，因此即便配置有误，也不会把一个没有保护的端口暴露到公网上。

回环地址（`127.0.0.1`）无需配对即可访问；其他设备则必须先完成配对（见上文「安装与启动」）。配对不会授予任何额外权限——执行 `homebase pair` 本身就要求先能在这台机器上跑命令，而这正是 Homebase 所交出的能力。

**这套模型的前提是：宿主机不与他人共用。** 由于回环地址免凭证，机器上任何其他本地账号都可以访问 `http://127.0.0.1:1990`，拿到一个以 Homebase 运行用户身份执行的 shell。在个人 Mac 或自己的 Linux 机器上，这与他们本来就能拿到的 shell 没有区别；但在多用户共用的主机上，这是一次本地提权。不要把 Homebase 跑在一台你不愿意把终端交给其他本地账号的机器上。

设备的会话凭证只以哈希形式保存在本机的 `devices.json` 中，明文的一次性令牌和会话密钥只出现一次——分别是 CLI 的标准输出和浏览器的 cookie。撤销某台设备的访问权限，可以在 Settings 页面（仅回环连接可见）完成。

## 从源码构建

```bash
git clone https://github.com/yanghanqing/homebase
cd homebase
go build -o homebase ./cmd/homebase
./homebase start
```
