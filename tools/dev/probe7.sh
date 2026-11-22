#!/system/bin/sh
# 稳定性 + 真实工具调用测试
DATA=/data/adb/agentcore
TOKEN=159357

sleep 8
echo "===STABLE_LOG_TAIL==="
tail -n 3 "$DATA/stdout.log"

echo "===ASK_TODAY==="
curl -s -m 90 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"今天运行了哪些任务？","sessionId":"probe2"}' \
  http://127.0.0.1:18790/v1/agent/message
echo

echo "===AUDIT_TAIL==="
tail -n 6 "$DATA/audit.jsonl" 2>/dev/null
