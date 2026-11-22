#!/system/bin/sh
CFG=/data/adb/agentcore/config.json
BASE=http://127.0.0.1:5700
CID=$(grep -o '"clientId": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
CSEC=$(grep -o '"clientSecret": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
TOKEN=$(curl -s -m 8 "$BASE/open/auth/token?client_id=$CID&client_secret=$CSEC" | grep -o '"token": *"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')

F=2026-11-01-22-33-00-681.log
P=fenxiang

for q in \
  "path=$P&file=$F" \
  "file=$F&path=$P" \
  "path=$P&name=$F" \
  "filename=$P/$F" \
  "path=$P&filename=$F"
do
  echo "=== $q ==="
  curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs/detail?$q" | head -c 220
  echo
done
