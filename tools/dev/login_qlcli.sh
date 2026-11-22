#!/system/bin/sh
export PATH=/data/adb/modules/qinglong/bin:/data/adb/ap/bin:/data/adb/ksu/bin:/data/adb/magisk:$PATH
ROOTFS=/data/alpine
BASEPATH="/opt/ql-cli/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
CFG=/data/adb/agentcore/config.json

CID=$(grep -o '"clientId": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
CSEC=$(grep -o '"clientSecret": *"[^"]*"' "$CFG" | head -1 | sed 's/.*: *"//;s/"$//')
echo "CID_LEN=${#CID} CSEC_LEN=${#CSEC}"

run() {
  rurima r -w -p -N -S -A -W /ql -e "PATH" "$BASEPATH" "$ROOTFS" /bin/sh -c "$1"
}

echo "=== login (env creds) ==="
run "QL_CLIENT_ID=$CID QL_CLIENT_SECRET=$CSEC ql login --url http://127.0.0.1:5700 --json" 2>&1 | tail -10

echo "=== auth status ==="
run 'ql auth status --json' 2>&1 | tail -14
