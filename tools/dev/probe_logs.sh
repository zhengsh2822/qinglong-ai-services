#!/system/bin/sh
# 探测青龙日志接口：列表 / 指定文件读取
CFG=/data/adb/agentcore/config.json
BASE=http://127.0.0.1:5700

CID=$(grep -o '"clientId": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
CSEC=$(grep -o '"clientSecret": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')

TOKEN=$(curl -s -m 8 -X POST "$BASE/open/auth/token?client_id=$CID&client_secret=$CSEC" \
  | grep -o '"token": *"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')
[ -z "$TOKEN" ] && TOKEN=$(grep -o '"token": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')

echo "TOKEN_LEN=${#TOKEN}"

echo "=== /open/logs ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs" | head -c 600
echo
echo "=== /open/logs?path= ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs?path=" | head -c 600
echo
echo "=== /open/logs?path=好奇车生活_330 ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs?path=%E5%A5%BD%E5%A5%87%E8%BD%A6%E7%94%9F%E6%B4%BB_330" | head -c 800
echo
