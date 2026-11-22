#!/usr/bin/env bash
# 热更新：推新二进制/脚本/WebUI 到手机并重启服务，不重装模块、不重启手机。
# 用法：bash tools/deploy.sh
set -e

# Windows Git Bash 会把 /data/local/tmp 当本地路径转换，必须禁用
export MSYS_NO_PATHCONV=1

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# adb 是 Windows 程序，push 的源路径必须是 F:/... 形式（MSYS 的 /f/... 它不认）
ROOT_WIN="$(cygpath -m "$ROOT" 2>/dev/null || printf '%s' "$ROOT")"
ADB="${ADB:-adb}"
MOD=/data/adb/modules/ksu_agent_core

if ! $ADB devices | awk '$2=="device"' | grep -q .; then
  echo "没有可用设备（adb devices 为空），先连上手机再执行"
  exit 1
fi

echo "[1/3] push"
$ADB push "$ROOT_WIN/bin/agentcore" /data/local/tmp/agentcore >/dev/null
$ADB push "$ROOT_WIN/service.sh" /data/local/tmp/service.sh >/dev/null
$ADB push "$ROOT_WIN/webroot/index.html" /data/local/tmp/index.html >/dev/null
$ADB push "$ROOT_WIN/action.sh" /data/local/tmp/action.sh >/dev/null
$ADB push "$ROOT_WIN/module.prop" /data/local/tmp/module.prop >/dev/null

echo "[2/4] stop old process"
$ADB shell "su -c 'kill \$(pidof agentcore) 2>/dev/null; sleep 1'" || true

echo "[3/4] install to module dir"
$ADB shell "su -c 'cp /data/local/tmp/agentcore $MOD/bin/agentcore && chmod 755 $MOD/bin/agentcore \
  && cp /data/local/tmp/service.sh $MOD/service.sh && chmod 755 $MOD/service.sh \
  && cp /data/local/tmp/index.html $MOD/webroot/index.html \
  && cp /data/local/tmp/action.sh $MOD/action.sh && chmod 755 $MOD/action.sh \
  && cp /data/local/tmp/module.prop $MOD/module.prop'"

echo "[4/4] restart service"
$ADB shell "su -c 'sh $MOD/action.sh'"
