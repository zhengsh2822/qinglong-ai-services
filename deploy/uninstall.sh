#!/usr/bin/env bash
# 卸载 Qinglong AI Services（服务器版）。默认保留配置与日志，加 --purge 连数据一起删。
set -euo pipefail

PREFIX="/opt/qinglong-ai-services"
SERVICE="qinglong-ai"
PURGE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX="$2"; shift 2 ;;
    --service) SERVICE="$2"; shift 2 ;;
    --purge) PURGE=1; shift ;;
    -h|--help)
      echo "用法：sudo bash uninstall.sh [--prefix DIR] [--service NAME] [--purge]"
      exit 0 ;;
    *) echo "未知参数：$1"; exit 1 ;;
  esac
done

[ "$(id -u)" -eq 0 ] || { echo "请用 root 运行"; exit 1; }

if command -v systemctl >/dev/null 2>&1 && [ -f "/etc/systemd/system/$SERVICE.service" ]; then
  echo "停止并移除服务：$SERVICE"
  systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$SERVICE.service"
  systemctl daemon-reload
fi

if [ "$PURGE" -eq 1 ]; then
  echo "删除整个目录：$PREFIX（配置、审计、日志都不保留）"
  rm -rf "$PREFIX"
else
  echo "删除程序文件，保留配置与日志：$PREFIX"
  rm -rf "$PREFIX/bin" "$PREFIX/webroot"
  echo "要彻底清理：rm -rf $PREFIX"
fi

echo "已卸载。"
