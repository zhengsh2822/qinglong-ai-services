#!/system/bin/sh
CFG=/data/adb/agentcore/config.json
BASE=http://127.0.0.1:5700
CID=$(grep -o '"clientId": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
CSEC=$(grep -o '"clientSecret": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
TOKEN=$(curl -s -m 8 "$BASE/open/auth/token?client_id=$CID&client_secret=$CSEC" | grep -o '"token": *"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')

echo "=== read with ?path= ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" \
  "$BASE/open/logs/2026-11-01-22-33-00-681.log?path=fenxiang" | tail -c 500
echo
echo "=== encode slash ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" \
  "$BASE/open/logs/fenxiang%2F2026-11-01-22-33-00-681.log" | head -c 200
echo
