#!/system/bin/sh
# 主启动器：由 ksud 在 late_start 以 root 执行。
# KernelSU 的 su context 是 u:r:ksu:s0，initrc 里写 u:r:su:s0 会让 init exec 失败（退出码 127），
# 因此不依赖 initrc，改为在这里起一个脱离会话的守护循环。
MODDIR=${0%/*}
DATA=/data/adb/agentcore

TZ_VALUE=$(getprop persist.sys.timezone 2>/dev/null)
[ -n "$TZ_VALUE" ] && export TZ="$TZ_VALUE"

mkdir -p "$DATA"
chmod 700 "$DATA" 2>/dev/null

# 等系统启动完成（最多约 2 分钟），避免网络/面板未就绪
i=0
while [ "$(getprop sys.boot_completed)" != "1" ]; do
  sleep 2
  i=$((i + 1))
  [ "$i" -gt 60 ] && break
done

# 已有实例则不重复启动
if pidof agentcore >/dev/null 2>&1; then
  exit 0
fi

# 守护进程单实例：避免重复执行 service.sh 时拉起多个 watchdog 抢端口
PIDFILE="$DATA/watchdog.pid"
if [ -f "$PIDFILE" ]; then
  OLD=$(cat "$PIDFILE" 2>/dev/null)
  if [ -n "$OLD" ] && kill -0 "$OLD" 2>/dev/null; then
    exit 0
  fi
fi

# 守护循环：仅在没有运行实例时才启动；setsid 脱离会话，避免随父进程回收
LOOP="while true; do if ! pidof agentcore >/dev/null 2>&1; then '$MODDIR/run.sh'; fi; sleep 5; done"
if command -v setsid >/dev/null 2>&1; then
  setsid /system/bin/sh -c "$LOOP" >>"$DATA/watchdog.log" 2>&1 &
else
  nohup /system/bin/sh -c "$LOOP" >>"$DATA/watchdog.log" 2>&1 &
fi
echo $! >"$PIDFILE"
