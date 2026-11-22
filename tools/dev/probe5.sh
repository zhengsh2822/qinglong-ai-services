#!/system/bin/sh
# 部署新二进制（含 DNS 修复）并做端到端测试
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore
TOKEN=159357

stop agentcore 2>/dev/null
PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null
sleep 1

cp /data/local/tmp/agentcore "$MOD/bin/agentcore"
chmod 755 "$MOD/bin/agentcore"

/system/bin/sh "$MOD/service.sh"
sleep 3

echo "===PID==="
pidof agentcore

echo "===HEALTH==="
curl -s -m 3 -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18790/health
echo

echo "===CHAT==="
curl -s -m 60 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"你好，用一句话说明你能帮我做什么","sessionId":"probe"}' \
  http://127.0.0.1:18790/v1/agent/message
echo

echo "===LOG==="
tail -n 8 "$DATA/stdout.log"
