#!/system/bin/sh
# 临时诊断脚本：绕过 init 手动拉起 agentcore，验证二进制/配置/HTTP
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore
TOKEN=159357

echo "===STOP INIT==="
stop agentcore 2>&1
PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null
sleep 1

echo "===START MANUAL==="
rm -f "$DATA/manual.log"
nohup "$MOD/bin/agentcore" -config "$DATA/config.json" -audit "$DATA/audit.jsonl" \
  >>"$DATA/manual.log" 2>&1 &
sleep 2

echo "===LOG==="
tail -n 12 "$DATA/manual.log" 2>&1

echo "===PID==="
pidof agentcore 2>&1

echo "===PORT==="
ss -ltn 2>/dev/null | grep 18790 || echo NO_LISTEN

echo "===HEALTH==="
curl -s -m 3 -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18790/health 2>&1
echo

echo "===UNAUTH==="
curl -s -m 3 -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18790/health 2>&1

echo "===QL==="
curl -s -m 3 -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5700/ 2>&1
