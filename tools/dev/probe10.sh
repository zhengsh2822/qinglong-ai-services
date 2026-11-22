#!/system/bin/sh
TOKEN=159357
echo "=== ASK_HISTORY ==="
curl -s -m 150 -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"text":"好奇车生活 昨天 12 点那次运行的日志末尾汇总是什么？","sessionId":"probe_log"}' \
  http://127.0.0.1:18790/v1/agent/message
echo
