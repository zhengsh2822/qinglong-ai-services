#!/system/bin/sh
# 开启局域网监听并输出手机 IP，供电脑侧验证
CONFIG=/data/adb/agentcore/config.json
TOKEN=159357

sed -i 's/127\.0\.0\.1:18790/0.0.0.0:18790/' "$CONFIG"

sh /data/adb/modules/ksu_agent_core/action.sh 2>&1 | tail -2
sleep 2

IP=$(ip -o -4 addr show wlan0 2>/dev/null | awk '{print $4}' | cut -d/ -f1)
if [ -z "$IP" ]; then
  IP=$(ip -o -4 addr show 2>/dev/null | grep -v '127.0.0.1' | head -1 | awk '{print $4}' | cut -d/ -f1)
fi

echo "PHONE_IP=$IP"
echo "LISTEN=$(grep -o '"listen"[^,]*' "$CONFIG")"
curl -s -m 3 -H "Authorization: Bearer $TOKEN" "http://$IP:18790/health" || echo "LOCAL_CURL_FAIL"
echo
