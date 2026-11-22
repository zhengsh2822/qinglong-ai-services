#!/usr/bin/env bash
# Qinglong AI Services 一键安装（Linux 服务器）
#
# 在线安装：
#   curl -fsSL https://raw.githubusercontent.com/zhengsh2822/qinglong-ai-services/main/deploy/install.sh \
#     | sudo bash -s -- --token 你的访问令牌
#
# 离线安装：把 agentcore-linux-amd64（或 arm64）和本脚本放同一目录，直接 sudo bash install.sh
#
# 装完就有：
#   - 常驻服务（systemd，Restart=always），开机自启
#   - 主服务 0.0.0.0:18790，提供 /v1/agent/* 与 /mcp（MCP 7×24 接入）
#   - 管理页 127.0.0.1:18791，浏览器配模型 / 青龙 / MCP，改完自动重启
set -euo pipefail

REPO="${QAS_REPO:-zhengsh2822/qinglong-ai-services}"
PREFIX="/opt/qinglong-ai-services"
PORT=18790
BIND="0.0.0.0"
WEBUI_PORT=18791
WEBUI_BIND="127.0.0.1"
TOKEN=""
PUBLIC_URL=""
RUN_USER="root"
VERSION="latest"
FROM_SOURCE=0
USE_SYSTEMD=1
SERVICE="qinglong-ai"

usage() {
  sed -n '2,15p' "$0" 2>/dev/null || true
  cat <<'EOF'

参数：
  --port 18790            主服务端口（/v1/agent、/mcp）
  --bind 0.0.0.0          主服务监听地址
  --webui-port 18791      管理页端口
  --webui-bind 127.0.0.1  管理页监听地址（建议本机 + 反代，不要直接暴露）
  --token XXX             访问令牌；不填自动生成
  --public-url https://ai.example.com   对外地址（走反代时填，用来自动拼 MCP 端点）
  --prefix /opt/qinglong-ai-services    安装目录
  --user root             运行用户
  --version v0.1.4        指定版本，默认 latest
  --from-source           用源码编译（需要 go 1.21+，适合不支持的架构）
  --no-systemd            只安装文件，不注册服务
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --bind) BIND="$2"; shift 2 ;;
    --webui-port) WEBUI_PORT="$2"; shift 2 ;;
    --webui-bind) WEBUI_BIND="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --public-url) PUBLIC_URL="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
    --user) RUN_USER="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --from-source) FROM_SOURCE=1; shift ;;
    --no-systemd) USE_SYSTEMD=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数：$1（--help 查看用法）"; exit 1 ;;
  esac
done

say() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*"; }
die() { printf '\033[31m[x]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "请用 root 运行（sudo bash install.sh）"

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "不支持的架构：$(uname -m)。可加 --from-source 自行编译（需 go 1.21+）" ;;
esac

gen_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
  else
    od -An -tx1 -N24 /dev/urandom | tr -d ' \n'
  fi
}

download() {
  src="$1"; dst="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 10 -o "$dst" "$src"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$dst" "$src"
  else
    die "需要 curl 或 wget"
  fi
}

[ -n "$TOKEN" ] || TOKEN="$(gen_token)"

say "安装目录：$PREFIX"
mkdir -p "$PREFIX/bin" "$PREFIX/webroot"
chmod 755 "$PREFIX" "$PREFIX/bin"

SCRIPT_DIR=""
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

BIN="$PREFIX/bin/agentcore"
LOCAL_BIN=""
for cand in "$SCRIPT_DIR/agentcore-linux-$ARCH" "$SCRIPT_DIR/dist/agentcore-linux-$ARCH"; do
  if [ -f "$cand" ]; then LOCAL_BIN="$cand"; break; fi
done

build_from_source() {
  say "从源码编译（$ARCH）"
  command -v go >/dev/null 2>&1 || die "没有 go，无法源码编译；可改用 Docker（docker compose up -d），或先把 agentcore-linux-$ARCH 放到本脚本同目录后重跑"
  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT
  download "https://github.com/$REPO/archive/refs/heads/main.tar.gz" "$TMP/src.tgz"
  tar -xzf "$TMP/src.tgz" -C "$TMP"
  SRC_ROOT="$(find "$TMP" -maxdepth 1 -type d -name 'qinglong-ai-services-*' | head -n1)"
  [ -n "$SRC_ROOT" ] || die "源码包结构异常"
  ( cd "$SRC_ROOT/src" && GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 \
      go build -trimpath -ldflags "-s -w" -o "$BIN" . )
}

if [ -n "$LOCAL_BIN" ]; then
  say "使用本地二进制：$LOCAL_BIN"
  install -m 755 "$LOCAL_BIN" "$BIN"
elif [ "$FROM_SOURCE" -eq 1 ]; then
  build_from_source
else
  if [ "$VERSION" = "latest" ]; then
    URL="https://github.com/$REPO/releases/latest/download/agentcore-linux-$ARCH"
  else
    URL="https://github.com/$REPO/releases/download/$VERSION/agentcore-linux-$ARCH"
  fi
  say "下载：$URL"
  if download "$URL" "$BIN"; then
    chmod 755 "$BIN"
  else
    rm -f "$BIN"
    warn "Release 下载失败（仓库还没有发布二进制，或指定版本不存在）。"
    if command -v go >/dev/null 2>&1; then
      warn "检测到 go，自动改用源码编译。"
      build_from_source
    else
      warn "可选做法："
      warn "  1) 先发版：git tag vx.y.z && git push origin vx.y.z"
      warn "     （.github/workflows/release.yml 会自动产出 agentcore-linux-amd64 / arm64）"
      warn "  2) 或在有 go 的机器上加 --from-source 重跑本脚本"
      warn "  3) 或用 Docker 部署：docker compose up -d"
      warn "  4) 或手动把 agentcore-linux-$ARCH 放到本脚本同目录后重跑"
      exit 1
    fi
  fi
fi

chmod 755 "$BIN"

# 管理页优先读磁盘 webroot，方便以后不重编二进制就换界面
if [ -f "$SCRIPT_DIR/../webroot/index.html" ]; then
  install -m 644 "$SCRIPT_DIR/../webroot/index.html" "$PREFIX/webroot/index.html"
fi

CONFIG="$PREFIX/config.json"
if [ -f "$CONFIG" ]; then
  say "已存在配置，保留：$CONFIG（要重配请先备份删除）"
else
  say "生成配置"
  cat > "$CONFIG" <<EOF
{
  "listen": "$BIND:$PORT",
  "authToken": "$TOKEN",
  "llm": {
    "baseUrl": "",
    "apiKey": "",
    "model": ""
  },
  "qinglong": {
    "baseUrl": "http://127.0.0.1:5700",
    "token": "",
    "clientId": "",
    "clientSecret": ""
  },
  "qqbot": {
    "enabled": false,
    "appId": "",
    "appSecret": ""
  },
  "mcp": {
    "enabled": true,
    "path": "/mcp",
    "token": ""
  },
  "webui": {
    "enabled": true,
    "listen": "$WEBUI_BIND:$WEBUI_PORT",
    "token": "",
    "publicUrl": "$PUBLIC_URL"
  },
  "maxRounds": 4
}
EOF
  chmod 600 "$CONFIG"
fi

if [ "$USE_SYSTEMD" -eq 1 ]; then
  command -v systemctl >/dev/null 2>&1 || die "没有 systemd，请加 --no-systemd 自行用 supervisor/nohup 启动"
  say "写入 systemd 服务：$SERVICE"
  cat > "/etc/systemd/system/$SERVICE.service" <<EOF
[Unit]
Description=Qinglong AI Services
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$RUN_USER
WorkingDirectory=$PREFIX
# sh -c + exec：日志落到文件（管理页要 tail），信号直达进程
ExecStart=/bin/sh -c 'exec $PREFIX/bin/agentcore -config $PREFIX/config.json -audit $PREFIX/audit.jsonl -log $PREFIX/stdout.log >> $PREFIX/stdout.log 2>&1'
Restart=always
RestartSec=2
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "$SERVICE" >/dev/null 2>&1 || true
  systemctl restart "$SERVICE"

  say "等待服务就绪"
  ok=0
  for _ in $(seq 1 20); do
    if curl -fsS --max-time 2 "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then ok=1; break; fi
    sleep 1
  done
  if [ "$ok" -eq 1 ]; then
    say "服务已启动"
  else
    warn "服务未响应，看看日志：journalctl -u $SERVICE -n 50 --no-pager"
  fi
else
  warn "未注册 systemd。手动启动："
  echo "  $PREFIX/bin/agentcore -config $CONFIG -audit $PREFIX/audit.jsonl -log $PREFIX/stdout.log >> $PREFIX/stdout.log 2>&1 &"
fi

TOKEN_SHOW="$(grep -o '"authToken"[^,]*' "$CONFIG" | head -n1 | sed 's/.*: *"//; s/"$//')"
[ -n "$TOKEN_SHOW" ] || TOKEN_SHOW="$TOKEN"

echo
say "安装完成"
cat <<EOF

  管理页      http://$WEBUI_BIND:$WEBUI_PORT
              $([ "$WEBUI_BIND" = "127.0.0.1" ] && echo "本机端口，远程用隧道：ssh -L $WEBUI_PORT:127.0.0.1:$WEBUI_PORT root@你的服务器" || echo "已对外监听，建议用 Nginx/Caddy 加 HTTPS")
  API / MCP   http://$BIND:$PORT   （MCP 路径 /mcp）
  访问令牌    $TOKEN_SHOW
  配置文件    $CONFIG
  日志        $PREFIX/stdout.log    journalctl -u $SERVICE -f

  客户端接入（Cursor / Claude Code 等）在管理页「MCP 服务」里选格式直接复制；
  走了反向代理记得在配置里填 webui.publicUrl，否则端点地址会按请求 Host 猜。

EOF
