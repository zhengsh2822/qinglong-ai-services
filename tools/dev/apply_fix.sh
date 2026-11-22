#!/system/bin/sh
# 现场修复：停掉 init 循环 → 覆盖新脚本/WebUI → 起 watchdog → 自检
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore
TOKEN=159357

echo "===STOP INIT==="
stop agentcore 2>/dev/null
PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null
sleep 1

echo "===APPLY FILES==="
cp /data/local/tmp/service.sh "$MOD/service.sh" && echo "service.sh ok"
cp /data/local/tmp/boot-completed.sh "$MOD/boot-completed.sh" && echo "boot-completed.sh ok"
cp /data/local/tmp/index.html "$MOD/webroot/index.html" && echo "webroot ok"
chmod 755 "$MOD/service.sh" "$MOD/boot-completed.sh"
chmod 644 "$MOD/webroot/index.html"

echo "===START WATCHDOG==="
/system/bin/sh "$MOD/service.sh"
sleep 3

echo "===INIT==="
getprop init.svc.agentcore

echo "===PID==="
pidof agentcore

echo "===HEALTH==="
curl -s -m 3 -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18790/health
echo

echo "===CHAT==="
curl -s -m 45 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"你好，用一句话说明你能帮我做什么","sessionId":"probe"}' \
  http://127.0.0.1:18790/v1/agent/message
echo
