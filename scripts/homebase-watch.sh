#!/bin/zsh
# Homebase / tmux session 死亡取证。
#
# 纯只读：不改 homebase、tmux 或任何 session 的行为，只观察并落盘。
# 用来抓「浏览器里 session 突然 exit、必须 claude --resume」的真凶——
# agent 崩溃、WebSocket 断开、homebase 崩溃这三种情况已实测排除，
# 它们都不会销毁 session，所以真正的原因只会在真实运行中现形。
#
# 前台试跑: ./scripts/homebase-watch.sh
# 常驻:     见本文件末尾的 LaunchAgent 片段
# 日志:     ~/Library/Logs/homebase-watch.log

TMUX=${HOMEBASE_TMUX:-/opt/homebrew/bin/tmux}
LOG="${HOMEBASE_WATCH_LOG:-$HOME/Library/Logs/homebase-watch.log}"
SESSION=homebase
INTERVAL=${HOMEBASE_WATCH_INTERVAL:-10}

mkdir -p "$(dirname "$LOG")"
prev=""
prev_created=""

echo "[$(date '+%Y-%m-%d %H:%M:%S')] watcher 启动 (每 ${INTERVAL}s 一次)" >> "$LOG"

while true; do
  # session_created 是 session 的身份证：它一变，就说明旧 session 被销毁、
  # 新 session 被 `new-session -A` 静默重建了——这正是丢工作的那一刻。
  created=$($TMUX display-message -p -t $SESSION '#{session_created}' 2>/dev/null)
  srv=$($TMUX display-message -p -t $SESSION '#{pid}' 2>/dev/null)
  panes=$($TMUX list-panes -a -t $SESSION -F '#{window_index}:#{pane_pid}:#{pane_current_command}' 2>/dev/null | tr '\n' ',')
  hb=$(pgrep -f 'homebase serve' | tr '\n' ' ')
  cur="created=$created srv=$srv panes=$panes hb=$hb"

  if [[ "$cur" != "$prev" ]]; then
    ts=$(date '+%Y-%m-%d %H:%M:%S')
    {
      echo "[$ts] CHANGE"
      echo "  before: $prev"
      echo "  after : $cur"
    } >> "$LOG"

    if [[ -n "$prev_created" && -n "$created" && "$created" != "$prev_created" ]]; then
      {
        echo "  !!! SESSION 身份变更：旧 session 已销毁"
        echo "      old_created=$prev_created  new_created=$created"
        echo "  --- 最近 2 分钟 tmux / homebase 相关系统日志 ---"
        log show --last 2m --predicate \
          'process == "tmux" OR process == "homebase" OR eventMessage CONTAINS "tmux" OR eventMessage CONTAINS "homebase"' \
          --style compact 2>/dev/null | tail -40
        echo "  --- 最近的 jetsam / 崩溃报告 ---"
        ls -lt /Library/Logs/DiagnosticReports/ 2>/dev/null | grep -i jetsam | head -3
        ls -lt "$HOME/Library/Logs/DiagnosticReports/" 2>/dev/null | head -5
        echo "  --- 当前内存压力 ---"
        memory_pressure 2>/dev/null | tail -3
        echo "  --- 谁还连着 ssh ---"
        who
      } >> "$LOG"
    elif [[ -n "$prev_created" && -z "$created" ]]; then
      echo "  !!! session 当前不存在（tmux server 可能已死）" >> "$LOG"
    fi

    prev="$cur"
    [[ -n "$created" ]] && prev_created="$created"
  fi
  sleep "$INTERVAL"
done

# ---------------------------------------------------------------------------
# 常驻方式（把下面这段存成
#   ~/Library/LaunchAgents/com.yanghanqing.homebase-watch.plist
# 再 launchctl bootstrap gui/$(id -u) 该文件；卸载用 launchctl bootout）
#
# <?xml version="1.0" encoding="UTF-8"?>
# <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
#   "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
# <plist version="1.0"><dict>
#   <key>Label</key><string>com.yanghanqing.homebase-watch</string>
#   <key>ProgramArguments</key>
#   <array><string>/Users/yanghanqing/Developer/homebase/scripts/homebase-watch.sh</string></array>
#   <key>KeepAlive</key><true/>
#   <key>RunAtLoad</key><true/>
#   <key>ThrottleInterval</key><integer>30</integer>
# </dict></plist>
# ---------------------------------------------------------------------------
