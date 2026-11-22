#!/system/bin/sh
# 开机完成再检查一次：统一交给 service.sh（含 watchdog）
MODDIR=${0%/*}

if pidof agentcore >/dev/null 2>&1; then
  exit 0
fi

/system/bin/sh "$MODDIR/service.sh"
