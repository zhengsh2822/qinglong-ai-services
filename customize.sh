#!/system/bin/sh
# KernelSU 安装脚本：设置权限并准备数据目录（配置不随模块升级丢失）
SKIPUNZIP=0

ui_print "- KSU AI Agent Core 安装中"

set_perm_recursive "$MODPATH" 0 0 0755 0644
set_perm_recursive "$MODPATH/bin" 0 0 0755 0755
set_perm_recursive "$MODPATH/webroot" 0 0 0755 0644
set_perm_recursive "$MODPATH/initrc" 0 0 0755 0644
set_perm "$MODPATH/run.sh" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm "$MODPATH/boot-completed.sh" 0 0 0755
set_perm "$MODPATH/uninstall.sh" 0 0 0755
set_perm "$MODPATH/action.sh" 0 0 0755
[ -f "$MODPATH/bin/agentcore" ] && set_perm "$MODPATH/bin/agentcore" 0 0 0755

# 运行时数据与配置放在模块目录之外，升级/卸载模块不会丢
mkdir -p /data/adb/agentcore
chmod 700 /data/adb/agentcore
if [ ! -f /data/adb/agentcore/config.json ]; then
  cp "$MODPATH/default-config.json" /data/adb/agentcore/config.json
fi
chmod 600 /data/adb/agentcore/config.json

ui_print "- 配置：/data/adb/agentcore/config.json"
ui_print "- 重启后生效，或在 KernelSU 管理器里执行 Action/重启模块"
