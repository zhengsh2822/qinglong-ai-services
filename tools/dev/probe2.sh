#!/system/bin/sh
# 诊断二：把端口让给 init 服务，观察它能否稳定接管
DATA=/data/adb/agentcore
TOKEN=159357

PID=$(pidof agentcore 2>/dev/null)
[ -n "$PID" ] && kill "$PID" 2>/dev/null
sleep 1

start agentcore 2>&1
sleep 3

echo "===INIT_STATE==="
getprop init.svc.agentcore

echo "===PID==="
pidof agentcore 2>&1

echo "===LOG_TAIL==="
tail -n 6 "$DATA/stdout.log" 2>&1

echo "===HEALTH==="
curl -s -m 3 -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18790/health 2>&1
echo
