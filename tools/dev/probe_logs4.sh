#!/system/bin/sh
CFG=/data/adb/agentcore/config.json
BASE=http://127.0.0.1:5700
CID=$(grep -o '"clientId": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
CSEC=$(grep -o '"clientSecret": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
TOKEN=$(curl -s -m 8 "$BASE/open/auth/token?client_id=$CID&client_secret=$CSEC" | grep -o '"token": *"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')

FILE=fenxiang/2026-11-01-22-33-00-681.log
echo "=== HEAD ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs/$FILE" | head -c 300
echo
echo "=== TAIL ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs/$FILE" | tail -c 400
echo
