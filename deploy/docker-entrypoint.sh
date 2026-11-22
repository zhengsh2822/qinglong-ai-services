#!/bin/sh
# 容器入口：首次启动生成配置并打印访问令牌，之后直接复用 /data 里的配置。
set -e

DATA="${DATA_DIR:-/data}"
mkdir -p "$DATA"

if [ ! -f "$DATA/config.json" ]; then
  TOKEN="${QAS_TOKEN:-}"
  if [ -z "$TOKEN" ]; then
    TOKEN="$(od -An -tx1 -N24 /dev/urandom | tr -d ' \n')"
  fi
  cat > "$DATA/config.json" <<EOF
{
  "listen": "0.0.0.0:18790",
  "authToken": "$TOKEN",
  "llm": { "baseUrl": "", "apiKey": "", "model": "" },
  "qinglong": { "baseUrl": "http://127.0.0.1:5700", "token": "", "clientId": "", "clientSecret": "" },
  "qqbot": { "enabled": false, "appId": "", "appSecret": "" },
  "mcp": { "enabled": true, "path": "/mcp", "token": "" },
  "webui": { "enabled": true, "listen": "0.0.0.0:18791", "token": "", "publicUrl": "${QAS_PUBLIC_URL:-}" },
  "maxRounds": 4
}
EOF
  chmod 600 "$DATA/config.json"
  echo "[entrypoint] 已生成 $DATA/config.json"
  echo "[entrypoint] 访问令牌：$TOKEN"
fi

exec agentcore \
  -config "$DATA/config.json" \
  -audit "$DATA/audit.jsonl" \
  -log "$DATA/stdout.log" \
  -webroot /opt/qinglong/webroot
