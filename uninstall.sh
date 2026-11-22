#!/system/bin/sh
# 卸载：只停止进程，保留 /data/adb/agentcore 下的配置与审计，方便重装
pkill -f "bin/agentcore" 2>/dev/null
exit 0
