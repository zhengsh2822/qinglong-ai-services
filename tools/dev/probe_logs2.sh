#!/system/bin/sh
CFG=/data/adb/agentcore/config.json
BASE=http://127.0.0.1:5700

CID=$(grep -o '"clientId": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
CSEC=$(grep -o '"clientSecret": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')

echo "CID=[$CID] len=${#CID}"
echo "CSEC_LEN=${#CSEC}"

echo "=== token resp ==="
RESP=$(curl -s -m 8 -X POST "$BASE/open/auth/token?client_id=$CID&client_secret=$CSEC")
echo "${RESP}" | head -c 300
echo

TOKEN=$(echo "$RESP" | grep -o '"token": *"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')
echo "TOKEN_LEN=${#TOKEN}"

echo "=== /open/logs ==="
curl -s -m 8 -H "Authorization: Bearer $TOKEN" "$BASE/open/logs" | head -c 700
echo
