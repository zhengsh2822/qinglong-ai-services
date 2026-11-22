#!/system/bin/sh
# 验证确认卡片是否带任务名称
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore
TOKEN=159357

PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null
cp /data/local/tmp/agentcore "$MOD/bin/agentcore"
chmod 755 "$MOD/bin/agentcore"
sleep 8

echo "===PID==="
pidof agentcore

echo "===ASK_RUN==="
curl -s -m 90 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"运行一下品赞白名单","sessionId":"probe4"}' \
  http://127.0.0.1:18790/v1/agent/message
echo
