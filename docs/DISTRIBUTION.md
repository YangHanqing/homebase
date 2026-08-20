# Homebase 安装、发布与命令面

| 字段 | 值 |
| --- | --- |
| 状态 | Accepted，待施工 |
| 日期 | 2026-08-21 |
| 范围 | GitHub 发布、curl 安装、服务生命周期、CLI 与 UI 分工 |
| 不在本文 | 协议、tmux argv、auth/TLS 推导（见 `AGENT.md` / `DESIGN.md`） |

本文是施工说明：定命令面、服务生命周期和安装脚本的行为，不写代码。文件在 `docs/` 里，随仓库公开——不要往里写密钥、内网地址或私人主机名。

`AGENT.md` 仍是硬约束的唯一真相源。本文若与其冲突，以 `AGENT.md` 为准；本文引入的新 REST 路由，按 `CLAUDE.md` 的契约要在同一改动里写进 `AGENT.md`。

---

## 目标体验

用户没有 Go 工具链。一条命令装好，一条命令启动，一条命令拿到配对链接。配置走已经存在的 Settings UI，不在 CLI 再做一套。

```bash
curl -fsSL https://github.com/yanghanqing/homebase/releases/latest/download/install.sh | sh
homebase start
# 本机打开 http://127.0.0.1:1990
# 需要给别的设备用：Settings 里改 Access → homebase restart → homebase pair
homebase pair
```

以后 Homebrew 只替换第一行（`brew install homebase`），后面的词不变。

---

## 宿主范围

v1 **宿主**（跑 `homebase` 进程的那台机器）：

| OS | Arch | 服务 |
| --- | --- | --- |
| macOS | arm64、amd64 | launchd user agent |
| Linux | amd64、arm64 | systemd user unit |

v1 **不**做 Windows 宿主。产品是本机 tmux 的 PTY 桥，Windows 没有 tmux，`creack/pty` + `exec tmux` 过不去。Windows / iPad / 手机只当浏览器客户端。installer 在 Windows 上直接失败并说明这一点。

WSL 里跑 Linux 二进制是用户自己的事，不写进主安装路径。

---

## CLI 与 UI 怎么切

原则：**CLI 只做 UI 做不到的事。**

- 服务没起来时，浏览器不存在 → 启停必须是命令。
- Settings 已经能改 `access` 和 `trusted_ranges`，且只允许从 `127.0.0.1` 打开 → **不再提供 `homebase access`。**
- 配对的安全论证是「能在这台机器上执行一条命令」（或等价的本机存在）。`homebase pair` 留下：SSH 上去、没有本机 GUI 浏览器时，这是唯一入口。Settings 可以加一颗「生成配对链接」按钮（同样只在 loopback），那是方便项，不替代这条命令。

现有 `homebase devices` 也不进公开命令面。设备列表/吊销应进 Settings（同样 loopback-only）。实现期若 Settings 还没这块，内部子命令可以暂时留着，但 README 不写、`homebase` 无参数的帮助里不出现。

---

## 公开命令面

一共六条。无参数或 `help` 只列出这些。

| 命令 | 作用 | 退出后终端 |
| --- | --- | --- |
| `homebase start` | 写成系统服务并拉起，打印本机 URL 和下一步 | 立刻回到 prompt |
| `homebase stop` | 停服务，不删配置、不碰 tmux session | 回到 prompt |
| `homebase restart` | stop + start。Settings 改完 Access 后用这一条 | 回到 prompt |
| `homebase status` | 服务在不在跑、绑定地址、URL、access、是否需要配对、已配对设备数、tmux 找没找到、版本 | 回到 prompt |
| `homebase pair` | 打出一次性、10 分钟有效的登录 URL | 回到 prompt |
| `homebase version` | 打印构建版本（Releases 的 tag） | 回到 prompt |

调试用、不进 README 主路径：

| 命令 | 作用 |
| --- | --- |
| `homebase serve` | 当前台 HTTP 服务（现在无参数的行为）。给开发者和排错，不装 launchd/systemd |

flag 归属：`-listen` / `-port` / `-log-level` 只留在 `serve`（今天 `run()` 上那几个原样搬过去）。`-config` 在 `serve` / `start` / `restart` / `status` / `pair` 上都要有，且**必须**是同一个语义——`setup()` 已经保证了「所有子命令用同一份 config 重算 listen 结果，不去问正在跑的服务」，这条不要破坏。

退出码：`0` 成功；`2` 用法错误（flag 解析失败、未知子命令）**以及缺 tmux**；其余失败 `1`。缺 tmux 复用 `2` 是有意的：调用方只需要区分「环境没准备好，照着提示做还能救」和「真的炸了」。

`homebase version`：`-ldflags "-X main.version=..."` 注入；没注入时（`go build` 出来的开发二进制）回落到 `runtime/debug.ReadBuildInfo()` 的 vcs revision，打成 `dev (abc1234)`。不要打空字符串。

`homebase pair` **不需要重启服务**：`devices.Store` 每次读写前会比对文件戳重载（`reloadIfChangedLocked`），CLI mint 出来的 token，正在跑的服务立刻认。别为了「让服务看见新 token」去加重启逻辑。

**没有这些命令：**

| 现状 | 去向 |
| --- | --- |
| `homebase` 无参数 = 前台起服务 | 改为打印帮助 |
| `homebase access …` | Settings → Access。改完提示 `homebase restart` |
| `homebase devices …` | Settings → 已配对设备（待补 UI）。公开 CLI 不保留 |

不提供 `install` / `uninstall` / `update` 子命令。安装是 `install.sh`（以后是 brew）；升级是再跑一遍安装脚本；卸载以后再说。

`start` 成功时的 stdout 按档位分支，**默认 `local` 不打印 pairing URL**（`127.0.0.1` 的 pairing 链接手机打不开，会教错用户）：

```
Homebase is running.
Open  http://127.0.0.1:1990     (this machine only)

To reach this from another device:
  1. open http://127.0.0.1:1990/settings.html on this machine
  2. set Access to Trusted range (or LAN, if you understand the risk)
  3. homebase restart
  4. homebase pair
```

URL 里的端口来自 config，不要写死 `1990`；`local` 档位下 Settings 只能从本机开，所以第一行给完整可点的 loopback 链接，别写「open Settings」让用户自己找。「Trusted range」是 Settings 页上 `private` 档位的显示名（`web/settings.html`），文案要和 UI 对得上。

若当前档位已经需要配对、且还没有设备，`start` **额外 mint 一条 pairing URL** 打在后面，避免「start 完还要再记 pair」。`pair` 仍留给第二台设备、以及 start 当时还不需要配对的情况。

`status` 在服务没在跑时仍要能打印配置与「not running」，不要因为进程不在就失败到什么都看不到。现有实现（`runStatus`）已经是从 config 重算而非问服务，保留全部现有行 —— `access` / `bind` / `url` / `trusted range` / `pairing` / `devices` / `config` 以及两条 warning —— 只新增 `running`、`tmux`、`version` 和日志位置四行。

---

## tmux：安装脚本引导，启动时再拦一次

Homebase 没有 tmux 就不是产品。installer **必须**处理「机器上没装 tmux」，但 **不得**在 `curl | sh` 里擅自 `sudo apt` / `brew install`。

原因：

- `curl | sh` 的 stdin 是脚本本身，不是键盘。普通 `read` 提示无法工作；偷偷用 sudo 更危险。
- tmux 是系统包，不是我们编的。绑进 tarball 会碰到签名、libc、路径，全部不值得。
- 用户可能已经用 MacPorts / nix / 源码装过，只是不在当前 PATH。探测要和二进制自己找 tmux 的路径一致。

### 探测

与 `internal/tmux.LocalBinary`（`internal/tmux/tmux.go`）同一份名单，按顺序：

1. `PATH` 里的 `tmux`
2. `/opt/homebrew/bin/tmux`
3. `/usr/local/bin/tmux`
4. `/usr/bin/tmux`
5. `/opt/local/bin/tmux`

找到即视为已安装。不检查版本（现有代码也没有版本门）。

这份名单在 `install.sh`（shell）和 `start`（Go）里各存在一次。改动时两处必须一起改——Go 侧以 `LocalBinary` 为准，shell 侧照抄同一顺序，注释里互相指认。

### `install.sh` 在找不到 tmux 时

**仍然把 `homebase` 二进制装上**（PATH 那一步不要因为缺 tmux 而回滚），然后把下面整段打到 stderr，退出码用 `0` 以外的值（建议 `2`，与「下载失败」的 `1` 区分）：

```
homebase is installed, but tmux was not found.
Homebase is a tmux frontend — install tmux, then start it.

  macOS (Homebrew):     brew install tmux
  Debian / Ubuntu:      sudo apt install tmux
  Fedora:               sudo dnf install tmux
  Arch:                 sudo pacman -S tmux

Then:  homebase start
```

按 `uname` 和 `/etc/os-release` **只高亮本机那一行**，其它行可缩成一句 “other systems: …”。macOS 没有 brew 时不要假装 `brew install` 能用，改成：

```
  macOS: install Homebrew (https://brew.sh), then:  brew install tmux
  or:    https://github.com/tmux/tmux/wiki/Installing
```

禁止：

- 非交互地执行任何包管理器
- 为了提示而去 `sudo`
- 从 GitHub 拉一份 tmux 二进制塞进 `~/.local/bin`
- 缺 tmux 时仍打印「下一步：homebase start」当成功路径

若以后要用 `/dev/tty` 做「现在帮你跑 brew install tmux?」那是可选增强，v1 不做。引导 = 打印正确的一条命令，不是代装。

### `homebase start` 再拦一次

installer 退出码非零之后，用户可能先不管、直接 `start`，或 tmux 装完不在我们搜的路径里。`start` 在写 plist/unit **之前**用同一套探测；找不到则拒绝启动、打与 installer 相同的那几行、退出 `2`。不要让 launchd/systemd 反复拉起一个必失败的进程。

`serve` 同样拒绝。UI 里现有的 `enotmux` 只覆盖「服务已在跑、后来 tmux 被卸掉」这种边角；正常安装路径不该走到那儿。

`status` 单独一行 `tmux  <path>|not found`，方便排错。

---

## `start` / `stop` / `restart` 做什么

进程不自己 fork、不用 `nohup`、不 `&`。后台是 OS 的事。

### 共同规则：plist / unit 里写哪个二进制

**`start` 不拷贝二进制。** 用 `os.Executable()` + `filepath.EvalSymlinks` 解析出当前正在跑的这个 `homebase` 的绝对路径，直接写进 plist/unit 的 `ProgramArguments` / `ExecStart`。

理由：拷贝会让开发者在源码树里跑一次 `./homebase start` 就把 `~/.local/bin/homebase` 覆盖掉，这是「命令做了没说的事」。装到稳定路径是 `install.sh` 的职责，`start` 只负责把已经存在的那个路径登记给服务管理器。

argv 固定为 `<abs-path> serve`（**不是**无参数——见「公开命令面」，无参数将改成打印帮助）。除非用户显式传了 `-config`，否则不往 argv 里塞任何 flag：端口和 access 从 config 读，这样 Settings 改完 `restart` 就生效，不会出现「plist 里钉死的 `-port` 覆盖了 config」这种两个真相源。

> 迁移：README 现在教用户手写的 plist 调的是**无参数**的二进制。改成帮助之后，那份旧 plist 会变成「起来就打印帮助然后退出」，被 `KeepAlive` 反复拉起。`start` 每次都无条件重写 plist/unit（而不是「文件已存在就跳过」），并在 bootout→bootstrap 后按下面的就绪探测确认真的活了。README 里那段手写 plist 同批删掉，改成指 `homebase start`。

### macOS（launchd user agent）

- 二进制稳定路径：`~/.local/bin/homebase`（由 install.sh 放置）。
- plist：`~/Library/LaunchAgents/com.yanghanqing.homebase.plist`（label 保持现 README 已写的这个）。
- `start`：写 plist → `launchctl bootout gui/$UID/<label>`（忽略「不存在」的错误）→ `launchctl bootstrap gui/$UID <plist>` → 就绪探测 → 打印 URL。
- `stop`：`launchctl bootout gui/$UID/<label>`。bootout 会连 `KeepAlive` 一起卸掉本轮，不需要另外改 plist；不删 plist 文件（再 `start` 还能用）。
- `restart`：`stop` + `start`。不要用 `kickstart -k`——它不会重读被改过的 plist，而 `start` 现在每次都重写 plist。
- 日志：plist 的 `StandardOutPath` / `StandardErrorPath` 指向 `~/Library/Logs/homebase.log`。

开机自起：plist `RunAtLoad` + `KeepAlive`，第一次 `start` 写好即可，不要再要用户碰 `launchctl`。

关于 `codesign`：**不要无条件 `codesign -s - -f`。** Go 链接器产出的 darwin/arm64 二进制自带 ad-hoc 签名，tarball 解包和 `mv` 都不破坏它；真正会拦人的是 quarantine xattr，那一步在 install.sh 里做。而 `codesign` 属于 Xcode Command Line Tools，裸装的 macOS 上可能根本没有，无条件调用等于给干净系统凭空加一个依赖。若确实碰到 `OS_REASON_CODESIGNING`，才走「`codesign -v` 失败 → 尝试 `codesign -s - -f` → 仍失败就打印手动命令」这条兜底路径。

### Linux（systemd --user）

- unit：`~/.config/systemd/user/homebase.service`。`Type=simple`、`Restart=on-failure`、`WantedBy=default.target`。
- `start`：写 unit → `daemon-reload` → `enable --now` →（重写过 unit 时）`restart` → 就绪探测。
- `systemctl --user` 需要一个能连上的 user bus。SSH 进来时 `XDG_RUNTIME_DIR` 可能没设，命令会以 `Failed to connect to bus` 失败。`start` 先检查该变量，缺失就提示用 `machinectl shell` / 重新登录，或直接告诉用户跑 `export XDG_RUNTIME_DIR=/run/user/$(id -u)`，**不要**把这个 bus 错误当成「服务起不来」去报别的原因。
- 无头机（SSH 登上去装、没有图形登录）：`start` 发现 linger 未开时打印：

  ```
  sudo loginctl enable-linger $USER
  ```

  不代跑 sudo。不 enable linger 的话，SSH 断开 user session 结束，服务就没了。
- `stop` / `restart`：对应 `systemctl --user`。
- 日志：journal（`journalctl --user -u homebase`），`status` 里写这句。

### 「在不在跑」怎么判定

`start` 和 `status` 都需要这个答案，用同一个函数，判据只有一条：**向 `http://127.0.0.1:<port>/api/health` 发一个 1 秒超时的 GET，2xx 即为在跑。**

- `/api/health` 已经存在（`internal/api/health.go`，`GET /api/health`），且在 auth gate 之外，loopback 拿得到，不需要配对。
- 即使 access 是 `private`/`lan`（绑在 tailscale IP 或 `0.0.0.0`），也**只探 `127.0.0.1`**：`0.0.0.0` 覆盖 loopback；绑到具体 tailscale IP 时探不到，此时再退回问服务管理器（`launchctl print` / `systemctl --user is-active`）。
- 不要用「pid 文件」或「`pgrep homebase`」——前者要自己维护、会残留，后者会把 `homebase status` 自己和别人的同名进程算进去。

`start` 在 bootstrap/`enable --now` **之后**轮询这个探测，最多 ~3 秒（launchd 对崩溃循环有 10 秒节流，别把超时设得比它短还报「失败」）：

- 探到 → 打印 URL 和下一步。
- 超时 → 退出码 `1`，打印日志路径（macOS：`~/Library/Logs/homebase.log`；Linux：`journalctl --user -u homebase -n 50`），不要打印「Homebase is running」。

`start` 在写 plist/unit **之前**还要做一次探测，用于区分两种「端口已被占用」：

- 探到 2xx 且 body 能解出 `{"ok":true,"listen":"..."}` → 已经在跑，见下面的幂等约定。（`listen` 是服务**真正**绑着的地址，`HealthHandler` 已经在返回它。）
- 端口连得上但不是我们 → 打印「port 1990 is in use by something else」并退出 `1`。**不要**装完服务让 launchd 反复拉起一个必然 bind 失败的进程。

### 行为约定

- 已在跑再 `start`：不报错，打印当前 URL，当成功（但仍要重写 plist/unit，覆盖旧格式）。
- 没在跑再 `stop`：不报错。
- 从没装过服务就 `restart`：等价于 `start`，不报错。
- 不删 `~/.config/homebase/`，不 `tmux kill-session`。
- Settings 保存 Access **不**从 HTTP 请求里重启自己的进程（跟 launchd KeepAlive 打架）。页面文案改成明确的「保存完毕后，在这台机器的终端执行 `homebase restart`」。
- `access` 变更后若用户忘了 restart，`status` **要**标出来：`/api/health` 返回的 `listen` 与 `setup()` 用当前 config 重算出的 `res.Addr` 不一致 → 打一行 `warning  config changed since start — run 'homebase restart'`。两个值都是现成的，这不是「若不好做」的事项。

---

## `install.sh`

发布为 GitHub Release 资产，仓库里留同一份便于审计。只做下载与放置，不编译。

开头 `set -eu`（不要 `pipefail`，`sh` 不一定有）。所有下载进 `mktemp -d` 出来的目录，`trap ... EXIT` 清掉。

1. 认 OS/arch：`darwin/arm64`、`darwin/amd64`、`linux/amd64`、`linux/arm64`。`uname -m` 的 `x86_64`/`aarch64`/`arm64` 都要映射对。其它（含 Windows、`darwin` 以外的 BSD）失败并说明 v1 宿主范围。
2. 向 GitHub `releases/latest` 拉对应 tarball 和 `checksums.txt`，校验 sha256。失败则停，不要「校验不过也装」。校验工具两边不一样：macOS 是 `shasum -a 256`，Linux 是 `sha256sum`，两个都探，都没有就直接失败——**不要**「找不到校验工具就跳过校验」。
3. 解包，然后**原子替换**到 `~/.local/bin/homebase`（`mkdir -p`）：先解到同一文件系统上的临时文件、`chmod +x`、再 `mv` 覆盖。直接往目标路径写会在服务正在跑时踩 Linux 的 `ETXTBSY`，而 `mv` 是 rename，跑着的老进程继续用旧 inode，不受影响。尽量不 sudo；该目录写不进去再提示用户把 `~/.local/bin` 造出来，或用他们有写权限的 PATH 目录（支持 `HOMEBASE_INSTALL_DIR` 覆盖）。
4. macOS：在 `mv` **之前**对临时文件清 quarantine（`xattr -d com.apple.quarantine <tmp> 2>/dev/null || true`），避免 Gatekeeper 拦住。
5. 若安装目录不在 PATH：打印要加进 `~/.zshrc` / `~/.bashrc` 的那一行，不擅自改 rc。
6. tmux 探测；缺则引导（见上）并以退出码 `2` 结束。
7. 成功路径最后一行：`homebase start`。

支持 `HOMEBASE_VERSION=v0.2.0` 装指定 tag（不设则 `latest`）。这既是回滚手段，也是出问题时唯一能让用户复现的钮。

**资产名是 install.sh 和 GoReleaser 之间的契约**，写死在两边并在本文钉住：

```
homebase_<version>_<os>_<arch>.tar.gz      # 例：homebase_0.1.0_darwin_arm64.tar.gz
checksums.txt
install.sh
```

`<os>` ∈ `darwin|linux`，`<arch>` ∈ `amd64|arm64`，tarball 内只有 `homebase` 一个可执行文件（外加 LICENSE/README）。GoReleaser 的 `archives.name_template` 显式写成这个格式，不要吃默认值——默认值随版本变过，改了就是安装脚本 404。

不在脚本里装 launchd/systemd——那是 `homebase start` 的工作，installer 保持可在无服务权限的环境里只丢二进制。

可重复执行：升级 = 再跑一遍，覆盖二进制。已有的 plist/unit 下次 `restart` 才会用到新文件；install.sh 成功后加一句「若服务已在跑：`homebase restart`」。

---

## 发布

没有 Releases 之前，README 不写 curl。空链接比没有更糟。

1. LICENSE。
2. 本机冒烟：`CGO_ENABLED=0` 交叉编四个目标。
3. GoReleaser + tag 触发的 GitHub Actions：四个 tarball + `checksums.txt` + `install.sh`；`ldflags` 写入 `homebase version`。
4. README 主路径改成 curl → `start` → 本机打开 → Settings → `restart` → `pair`。`go build` 降到「从源码构建」。

Homebrew 不进 v1。有了稳定资产名和至少一个 tag 再接 formula；`brew services` 必须和 `homebase start` 共用同一套 label，禁止两套 plist。

---

## 用户路径（装完以后）

```text
install.sh
    │
    ├─ 无 tmux → 打印本机的安装命令，二进制已在 PATH，停
    │                 用户装完 tmux
    │                      ▼
    └─ 有 tmux ────────────────────────────────────┐
                                                   ▼
                                           homebase start
                                                   │
                          launchd / systemd user，立刻回 prompt
                                                   ▼
                                    本机浏览器 http://127.0.0.1:1990
                                                   │
                          只要本机用：结束（local，无需配对）
                                                   │
                          要给手机/笔记本：Settings 改 Access
                                                   ▼
                                           homebase restart
                                                   ▼
                                             homebase pair
                                      （把 URL 在目标设备上打开）
```

`pair` 只在这台宿主上跑。这是威胁模型，不做「打开网页自己领一台设备」的公网入口。

---

## Settings 还要补的（CLI 砍掉之后）

现在 Settings 有 Access 和 trusted ranges，没有设备管理。公开 CLI 不再管设备之后，这块必须在 UI 出现，否则吊销只能手改 `devices.json`。

- 已配对设备列表 + 吊销（loopback-only，与改 Access 同一页即可）。
- 保存 Access 后的文案指向 `homebase restart`，不要再写「重启 Homebase」这种没有具体命令的句子。`PUT /api/settings` 已经在返回 `restart_required: true`，前端照这个字段渲染，不要自己猜。
- 可选：本页一颗「生成配对链接」，调用与 `pair` 相同的 mint。v1 可以没有，有 `pair` 就够。

新增两条路由，挂在**和 `/api/settings` 完全相同的 `requireLoopbackPeer` 之后**（注意不是 `auth.RequireLoopbackHost`——两者的区别见 `internal/api/settings.go` 顶部注释；这里要的是检查 TCP 对端地址那个，否则 `private`/`lan` 档位下已配对的手机就能吊销别人）：

| 路由 | 行为 | 状态码 |
| --- | --- | --- |
| `GET /api/devices` | 列出已配对设备 | 200 |
| `DELETE /api/devices/{id}` | 吊销一台 | 204；id 不存在 404 |

响应字段只有 `id` / `name` / `created_at`。**绝不返回** `devices.json` 里的 secret hash，也不返回未兑换的 pending token——UI 不需要，泄露了就是把配对凭证送出去。序列化用一个专门的 response struct，不要直接 `json.Marshal(devices.Device)`：那样以后往 `Device` 上加字段会自动漏出去。

这两条是新 REST 路由，按 CLAUDE.md 的契约，**同一个改动里要更新 `AGENT.md` 的路由表和状态码**。

顺序保护：先上这两条路由和 UI，再删 `homebase devices` 子命令。中间态不能出现「CLI 删了、UI 没有、只能手改 `devices.json`」。

---

## 明确以后再做

- Homebrew tap / `brew services`
- `homebase update`
- Windows 宿主、`install.ps1`、WSL 官方文档
- installer 交互代装 tmux
- 从 UI 触发进程重启
- 卸载命令

---

## 实现顺序（未开工）

1. Releases 骨架：LICENSE、GoReleaser（钉死资产名）、四个目标、`install.sh`（含 tmux 引导、原子替换、sha256 校验）、`homebase version`。
2. `start` / `stop` / `restart` / `status` / `serve`；无参数改成帮助；macOS launchd + Linux systemd user；`start` 拒无 tmux；就绪探测与端口占用判定；README 删掉手写 plist 那段。
3. README 主路径改成 curl → `start` → 本机打开 → Settings → `restart` → `pair`；`go build` 降到「从源码构建」。README.zh.md 同步。
4. `GET/DELETE /api/devices` + Settings 设备列表 UI + restart 文案；同批更新 `AGENT.md` 路由表。
5. 删掉公开的 `homebase access` / `homebase devices`；`start` 按档位打印 URL / pairing。
6. 以后：brew。

1 和 2 必须同一批发出去：只有 1 的话，README 只能写「装完了，但启动方式还是老的手写 plist」，中间态比现在更差。3 依赖 1+2 都在（README 里的 curl 链接必须已经能 200）。5 必须排在 4 之后——理由见上一节末尾。

---

## 关键决定

1. **宿主 v1 = macOS + Linux。** Windows 只当浏览器。不为了「支持 Windows」去编一个跑不起来的 `.exe`。
2. **公开 CLI 六条：`start` `stop` `restart` `status` `pair` `version`。** 配置走 Settings。`access` 从命令面删除。
3. **`pair` 留在 CLI。** Settings 改不了「我在 SSH 里、没有本机浏览器」这件事；也符合「配对 = 能在这台机器上跑一条命令」。
4. **缺 tmux：引导，不代装。** installer 把二进制装上并打印本机的一条 tmux 安装命令；`start` 再拦一次。`curl | sh` 里不跑包管理器。
5. **后台是 launchd / systemd，不是 Go daemon。** `restart` 是 Settings 改 Access 之后的那条命令，替代今天文档里含糊的「重启」。
6. **默认 `local`，`start` 不打印 pairing URL。** 需要配对时再 `pair`，或在已经是 private/lan 且零设备时由 `start` 顺手 mint。
7. **先有 checksum 过的 Releases，再把 curl 写进 README。** Homebrew 不进第一次发布。
8. **`start` 不拷贝二进制，只登记 `os.Executable()` 的绝对路径**，argv 固定 `<abs> serve`、不带 flag。放置二进制是 installer 的事，两个真相源不要并存。
9. **「在不在跑」只由 `GET /api/health` 回答**，不用 pid 文件、不用 `pgrep`；`start` 前后各探一次，分别用于识别端口占用和确认真的起来了。
10. **升级用 `mv` 原子替换**，不原地覆盖——服务正在跑时原地写会踩 `ETXTBSY`。
11. **设备管理走 `requireLoopbackPeer`，响应里没有 secret。** 先有 UI 再删 CLI。
