#!/system/bin/sh
# KernelSU 模块页「执行」按钮：安装/更新后点一下即可生效，无需重启手机。
MODDIR=${0%/*}
DATA=/data/adb/agentcore

TZ_VALUE=$(getprop persist.sys.timezone 2>/dev/null)
[ -n "$TZ_VALUE" ] && export TZ="$TZ_VALUE"

mkdir -p "$DATA"
chmod 700 "$DATA" 2>/dev/null

# 结束旧实例；watchdog 会在 5 秒内用新二进制拉起
PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null

# service.sh 自带 watchdog 单实例锁，可重复调用（不会起第二个守护进程）
/system/bin/sh "$MODDIR/service.sh"

# 最多等 10 秒（watchdog 每 5 秒轮询一次，3 秒不够会误报失败）
i=0
NEWPID=""
while [ "$i" -lt 10 ]; do
  NEWPID=$(pidof agentcore 2>/dev/null)
  [ -n "$NEWPID" ] && break
  sleep 1
  i=$((i + 1))
done
if [ -n "$NEWPID" ]; then
  echo "agentcore 已启动（pid $NEWPID）"
  echo "监听：$(grep -o '"listen"[^,]*' "$DATA/config.json" 2>/dev/null)"
else
  echo "启动失败，日志：$DATA/stdout.log"
  tail -n 10 "$DATA/stdout.log" 2>/dev/null
fi
