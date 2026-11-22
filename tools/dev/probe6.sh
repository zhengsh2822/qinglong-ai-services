#!/system/bin/sh
# 清理重复 watchdog，部署带单实例锁的 service.sh，验证只有一个实例
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore
TOKEN=159357

stop agentcore 2>/dev/null
for p in $(pidof agentcore 2>/dev/null); do kill -9 "$p" 2>/dev/null; done
for p in $(ps -A -o PID,ARGS 2>/dev/null | grep "while true" | grep -v grep | awk '{print $1}'); do
  kill -9 "$p" 2>/dev/null
done
rm -f "$DATA/watchdog.pid"
sleep 1

cp /data/local/tmp/service.sh "$MOD/service.sh"
chmod 755 "$MOD/service.sh"

/system/bin/sh "$MOD/service.sh"
sleep 3

echo "===WATCHDOG_COUNT==="
ps -A -o PID,ARGS 2>/dev/null | grep "while true" | grep -v grep | wc -l

echo "===PID==="
pidof agentcore

echo "===HEALTH==="
curl -s -m 3 -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18790/health
echo

echo "===LOG_TAIL==="
tail -n 4 "$DATA/stdout.log"
