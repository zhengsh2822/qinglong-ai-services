#!/system/bin/sh
# 部署 auth token GET 修复并复测今日任务
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore
TOKEN=159357

PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null
cp /data/local/tmp/agentcore "$MOD/bin/agentcore"
chmod 755 "$MOD/bin/agentcore"

# 等 watchdog 用新二进制拉起
sleep 8

echo "===PID==="
pidof agentcore

echo "===HEALTH==="
curl -s -m 3 -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18790/health
echo

echo "===ASK_TODAY==="
curl -s -m 90 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"今天运行了哪些任务？","sessionId":"probe3"}' \
  http://127.0.0.1:18790/v1/agent/message
echo

echo "===LOG_TAIL==="
tail -n 4 "$DATA/stdout.log"
