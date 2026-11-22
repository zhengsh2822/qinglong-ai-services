#!/system/bin/sh
# 在青龙 proot（/data/alpine）里安装 @whyour/qinglong-cli 到独立 prefix /opt/ql-cli
export PATH=/data/adb/modules/qinglong/bin:/data/adb/ap/bin:/data/adb/ksu/bin:/data/adb/magisk:$PATH
ROOTFS=/data/alpine
BASEPATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/ql/data/dep_cache/node"

echo "=== rurima ==="
which rurima || { echo "rurima not found"; exit 1; }

echo "=== container node/npm ==="
rurima r -w -p -N -S -A -W /ql -e "PATH" "$BASEPATH" "$ROOTFS" /bin/sh -c 'node -v; npm -v' 2>&1 | tail -4

echo "=== install qinglong-cli ==="
rurima r -w -p -N -S -A -W /ql -e "PATH" "$BASEPATH" "$ROOTFS" /bin/sh -c \
  'npm install -g --prefix /opt/ql-cli --registry=https://registry.npmmirror.com @whyour/qinglong-cli@0.1.3' 2>&1 | tail -18

echo "=== verify files ==="
ls -l "$ROOTFS/opt/ql-cli/bin/ql" 2>&1
grep -E '"name"|"version"' "$ROOTFS/opt/ql-cli/lib/node_modules/@whyour/qinglong-cli/package.json" 2>/dev/null | head -4

echo "=== verify run ==="
rurima r -w -p -N -S -A -W /ql -e "PATH" "/opt/ql-cli/bin:$BASEPATH" "$ROOTFS" /bin/sh -c 'ql --help' 2>&1 | head -12
