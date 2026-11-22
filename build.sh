#!/usr/bin/env bash
# 交叉编译并打包：
#   1) KSU 模块 zip（内含 android/arm64 二进制 + webroot）
#   2) 服务器用 linux 二进制（amd64 / arm64），供一键安装脚本与 Release 使用
set -e
ROOT="$(cd "$(dirname "$0")" && pwd)"
VERSION="$(grep '^version=' "$ROOT/module.prop" | cut -d= -f2)"
OUT_DIR="$(cd "$ROOT/.." && pwd)"
DIST="$ROOT/dist"
ZIP="$OUT_DIR/apk_output/ksu_agent_core_v${VERSION}.zip"

LDFLAGS="-s -w -X main.serviceVersion=$VERSION"
ZIP_EXCLUDES=("src/*" "dist/*" "deploy/*" "tools/*" "*.zip" "build.sh" "pack.py" \
  "bin/.gitkeep" "Dockerfile" "docker-compose.yml" ".gitignore" ".github/*")

mkdir -p "$DIST" "$OUT_DIR/apk_output"

echo "[1/5] 同步 WebUI 到内嵌目录（服务器模式靠它提供页面）"
cp "$ROOT/webroot/index.html" "$ROOT/src/internal/webui/index.html"

cd "$ROOT/src"

echo "[2/5] build android/arm64 (KSU 模块, version=$VERSION)"
GOOS=android GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$ROOT/bin/agentcore" .

echo "[3/5] build linux/amd64 (服务器)"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/agentcore-linux-amd64" .

echo "[4/5] build linux/arm64 (服务器 / 树莓派)"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/agentcore-linux-arm64" .

echo "[5/5] chmod + package"
chmod 755 "$ROOT/bin/agentcore" "$DIST/agentcore-linux-amd64" "$DIST/agentcore-linux-arm64"
chmod 755 "$ROOT/run.sh" "$ROOT/service.sh" "$ROOT/action.sh" \
  "$ROOT/boot-completed.sh" "$ROOT/uninstall.sh" "$ROOT/customize.sh" 2>/dev/null || true
chmod 755 "$ROOT"/deploy/*.sh 2>/dev/null || true

rm -f "$ZIP"
cd "$ROOT"
# 统一走 pack.py：zip -x 的排除规则在 Git Bash / 不同 Info-ZIP 版本下不一致，
# 会把 tools、dev 之类的开发文件带进模块包。python 的过滤规则是确定的。
PY=""
for cand in python3 python; do
  if command -v "$cand" >/dev/null 2>&1; then PY="$cand"; break; fi
done
if [ -n "$PY" ]; then
  "$PY" "$ROOT/pack.py"
elif command -v zip >/dev/null 2>&1; then
  zip -r "$ZIP" . -x "${ZIP_EXCLUDES[@]}" >/dev/null
  echo "built: $ZIP"
else
  echo "未找到 python / zip，请安装其中之一后重试"
  exit 1
fi

echo
echo "KSU 模块 : $ZIP"
echo "服务器版 : $DIST/agentcore-linux-amd64"
echo "服务器版 : $DIST/agentcore-linux-arm64"
