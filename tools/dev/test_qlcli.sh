#!/system/bin/sh
export PATH=/data/adb/modules/qinglong/bin:$PATH
echo "=== qlcli version/help ==="
qlcli --help 2>&1 | head -4
echo "=== qlcli task list ==="
qlcli task list --search 品赞 --json 2>&1 | head -c 300
echo
