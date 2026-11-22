#!/system/bin/sh
P=/data/alpine/opt/ql-cli/lib/node_modules/@whyour/qinglong-cli

echo "=== pkg files ==="
ls -1 "$P" 2>&1 | head -14

echo "=== skills dir ==="
ls -R "$P/skills" 2>/dev/null | head -24

echo "=== api routes ==="
export PATH=/data/adb/modules/qinglong/bin:$PATH
qlcli api routes --json 2>&1 | head -c 400
echo
