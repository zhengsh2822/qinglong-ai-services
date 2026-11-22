#!/system/bin/sh
# init 服务入口：前台 exec，进程退出后由 init 自动重启
MODDIR=${0%/*}
DATA=/data/adb/agentcore
BIN="$MODDIR/bin/agentcore"
CONFIG="$DATA/config.json"
AUDIT="$DATA/audit.jsonl"

mkdir -p "$DATA"
chmod 700 "$DATA" 2>/dev/null

if [ ! -x "$BIN" ]; then
  echo "agentcore binary missing: $BIN" >&2
  exit 1
fi

if [ ! -f "$CONFIG" ]; then
  cp "$MODDIR/default-config.json" "$CONFIG" 2>/dev/null
fi
chmod 600 "$CONFIG" 2>/dev/null

# init 服务环境没有 TZ，不设置会让 Go 回落到 UTC（日志/审计时间差 8 小时）
TZ_VALUE=$(getprop persist.sys.timezone 2>/dev/null)
[ -n "$TZ_VALUE" ] && export TZ="$TZ_VALUE"

# 保留启动输出，便于 WebUI 排查
exec "$BIN" -config "$CONFIG" -audit "$AUDIT" >>"$DATA/stdout.log" 2>&1
