#!/system/bin/sh
export PATH=/data/adb/modules/qinglong/bin:/data/adb/ap/bin:/data/adb/ksu/bin:/data/adb/magisk:$PATH
ROOTFS=/data/alpine
BASEPATH="/opt/ql-cli/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

run() {
  rurima r -w -p -N -S -A -W /ql -e "PATH" "$BASEPATH" "$ROOTFS" /bin/sh -c "$1"
}

echo "=== ql task list (first 400 chars) ==="
run 'ql task list --json' 2>&1 | head -c 400
echo

echo "=== ql system (first 300 chars) ==="
run 'ql system --json' 2>&1 | head -c 300
echo

echo "=== install host wrapper /data/adb/modules/qinglong/bin/qlcli ==="
cat > /data/adb/modules/qinglong/bin/qlcli <<'WRAP'
#!/system/bin/sh
export PATH=/data/adb/modules/qinglong/bin:/data/adb/ap/bin:/data/adb/ksu/bin:/data/adb/magisk:$PATH
exec rurima r -w -p -N -S -A -W /ql -e "PATH" "/opt/ql-cli/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" /data/alpine /bin/sh -c "ql $*"
WRAP
chmod 755 /data/adb/modules/qinglong/bin/qlcli
echo "ok: $(ls -l /data/adb/modules/qinglong/bin/qlcli)"
