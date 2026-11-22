#!/system/bin/sh
MOD=/data/adb/modules/ksu_agent_core
DATA=/data/adb/agentcore

kill $(pidof agentcore 2>/dev/null) 2>/dev/null
sleep 1

echo "=== A: sh run.sh ==="
/system/bin/sh "$MOD/run.sh" &
sleep 2
echo "pid=$(pidof agentcore 2>/dev/null)"
tail -n 2 "$DATA/stdout.log" 2>&1

echo "=== B: service.sh ==="
kill $(pidof agentcore 2>/dev/null) 2>/dev/null
sleep 1
/system/bin/sh "$MOD/service.sh" 2>&1
sleep 3
echo "pid=$(pidof agentcore 2>/dev/null)"
tail -n 2 "$DATA/stdout.log" 2>&1

echo "=== C: 直跑二进制对照 ==="
kill $(pidof agentcore 2>/dev/null) 2>/dev/null
sleep 1
"$MOD/bin/agentcore" -config "$DATA/config.json" -audit "$DATA/audit.jsonl" >>"$DATA/direct.log" 2>&1 &
sleep 2
echo "pid=$(pidof agentcore 2>/dev/null)"
tail -n 2 "$DATA/direct.log" 2>&1
