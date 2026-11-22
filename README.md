# Qinglong AI Services

青龙面板的常驻 AI 服务：一套二进制、一份配置，既能装成 **KernelSU 模块**跑在手机上，
也能**一键部署到云服务器**。对外提供 OpenAI 兼容的 Agent 接口、7×24 的 MCP 端点，
以及一个不用装 App 就能改配置的 Web 管理页。

- 语言：Go（纯标准库 + 内嵌 WebUI，编译出静态二进制，无运行时依赖）
- 接口：`POST /v1/agent/message`、`POST /v1/agent/confirm`、`POST /mcp`（MCP Streamable HTTP）
- 模型：任意 OpenAI 兼容端点（也支持 Anthropic / OpenAI Responses 协议）
- 青龙：只走官方 OpenAPI `/open/*`，不调 CLI、不拼 shell
- 安全：写操作强制人工确认、令牌鉴权、配置 0600、JSONL 审计

## 两种部署形态

| | KernelSU 模块 | 云服务器 / Docker |
|---|---|---|
| 安装 | 刷模块 zip，管理器里点「执行」 | `deploy/install.sh` 一键装，或 `docker compose up -d` |
| 守护 | watchdog 循环（5s 探活） | systemd `Restart=always` / 容器 `restart: unless-stopped` |
| 管理页 | 管理器自带 WebUI | 进程自带，浏览器开 `:18791` |
| 页面怎么改配置 | `ksu.exec()` 跑 root 命令 | `/api/*` + `Authorization: Bearer` |
| 适用 | 手机本地、跟青龙面板同机 | 公网 7×24、给多台设备/多个客户端共用 |

两种形态共用同一份 `webroot/index.html`：页面启动时自己判断有没有 `ksu.exec()`，
有就走 shell，没有就走 HTTP API。

## 快速开始（服务器）

```bash
# 一条命令装好：二进制 + 配置 + systemd 服务
curl -fsSL https://raw.githubusercontent.com/zhengsh2822/qinglong-ai-services/main/deploy/install.sh \
  | sudo bash -s -- --token 你的访问令牌
```

装完会打印管理页地址、令牌和 MCP 端点。管理页默认只听 `127.0.0.1:18791`：

> 脚本默认从 GitHub Release 下载二进制。仓库还没发版时会自动改用源码编译（需要 go）；
> 两者都没有就按提示换 Docker 或手动放二进制。发版只需打 tag：
> `git tag v0.1.4 && git push origin v0.1.4`（CI 会产出 `agentcore-linux-amd64/arm64`）。

```bash
# 本机浏览器访问（推荐，配 SSH 隧道）
ssh -L 18791:127.0.0.1:18791 root@你的服务器
# 然后打开 http://127.0.0.1:18791
```

要直接对外就在安装时加 `--webui-bind 0.0.0.0`（必须同时有令牌），或者用 Nginx / Caddy
反代并配 `--public-url https://ai.example.com`。

常用参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--port` | 18790 | API / MCP 端口 |
| `--bind` | 0.0.0.0 | API / MCP 监听地址 |
| `--webui-port` | 18791 | 管理页端口 |
| `--webui-bind` | 127.0.0.1 | 管理页监听地址 |
| `--token` | 自动生成 | 访问令牌（写入 `authToken`） |
| `--public-url` | 空 | 对外地址，反代时填，用来拼 MCP 端点 |
| `--prefix` | `/opt/qinglong-ai-services` | 安装目录 |
| `--version` | latest | 指定 Release 版本 |
| `--from-source` | 关 | 用源码编译（不支持的架构时用） |
| `--no-systemd` | 关 | 只放文件，不注册服务 |

卸载：`sudo bash deploy/uninstall.sh`（加 `--purge` 连配置日志一起删）。

### Docker

```bash
docker compose up -d          # 首次启动打印访问令牌
docker compose logs -f
```

`docker-compose.yml` 默认把管理页绑到宿主机回环（`127.0.0.1:18791`），只对外暴露 18790。

## 快速开始（KernelSU）

1. `./build.sh` 产出 `apk_output/ksu_agent_core_v<version>.zip`，放进手机；
2. KernelSU 管理器 → 模块 → 从本地安装；
3. **不用重启手机**：回模块页点「执行」，服务立即启动；
4. 点「WebUI」填模型 / 青龙 / MCP，保存即重启服务进程。

开发期热更新（推二进制 + 脚本 + 页面，不重装模块、不重启手机）：

```bash
bash tools/deploy.sh
```

## 管理页

一个页面，两套后端：

- **服务状态**：运行中/未运行、PID、服务器时间、配置文件路径
- **服务**：监听地址、访问令牌、最大推理轮次
- **MCP 服务（7×24）**：开关、端点路径、独立令牌、客户端地址（可复制）
- **客户端配置片段**：内置 19 种客户端格式（Cursor / Claude Code / Codex / GitHub Copilot /
  Cherry Studio / Cline / 通义灵码 / Gemini CLI / Windsurf / Zed / Continue / Goose /
  Claude Desktop / 通用 JSON / 仅 URL），自动带上 `Authorization` 头
- **模型**、**青龙面板**、**QQ 机器人**
- **日志 / 审计**：页内查看最近输出，不用登服务器

保存配置后进程会主动退出，由守护方拉起（systemd / watchdog / 容器），
配置里页面不认识的字段（`skill`、`dns`、扩展项）会原样保留，不会被覆盖掉。

## 配置

默认 `/opt/qinglong-ai-services/config.json`（服务器）或 `/data/adb/agentcore/config.json`（KSU），权限 0600：

```json
{
  "listen": "0.0.0.0:18790",
  "authToken": "访问令牌",
  "llm": {
    "baseUrl": "https://your-api-host/v1",
    "apiKey": "your-api-key",
    "model": "your-model-name"
  },
  "qinglong": {
    "baseUrl": "http://127.0.0.1:5700",
    "token": "",
    "clientId": "",
    "clientSecret": ""
  },
  "qqbot": { "enabled": false, "appId": "", "appSecret": "" },
  "mcp": { "enabled": true, "path": "/mcp", "token": "" },
  "webui": {
    "enabled": true,
    "listen": "127.0.0.1:18791",
    "token": "",
    "publicUrl": ""
  },
  "maxRounds": 4
}
```

| 字段 | 说明 |
|---|---|
| `listen` | API / MCP 监听地址；对外必须同时配 `authToken` |
| `authToken` | 全局访问令牌；为空时只接受本机回环 |
| `llm.protocol` | `openai_chat`（默认）/ `anthropic` / `openai_responses` |
| `llm.temperature` | 省略或 `null` 时不传（部分思考模型不接受自定义值） |
| `qinglong.token` | 青龙应用 Token；与 `clientId`+`clientSecret` 二选一 |
| `mcp.enabled` / `path` / `token` | MCP 端点开关、路径（自动归一化）、独立令牌 |
| `webui.enabled` | 是否由进程自带管理页；KSU 模块用管理器页面，保持 `false` |
| `webui.listen` | 管理页监听地址，默认 `127.0.0.1:18791` |
| `webui.token` | 管理页独立令牌；空则沿用 `authToken` |
| `webui.publicUrl` | 对外地址，反代时必填，用于拼 MCP 端点 |

## MCP 接入

Core 常驻，App 退出后 MCP 端点仍可用 —— Cursor / Claude Code / Cherry Studio 的长期接入走这里。

管理页选中客户端后直接复制片段，例如：

```json
{
  "mcpServers": {
    "qinglong_core": {
      "type": "http",
      "url": "https://ai.example.com/mcp",
      "headers": { "Authorization": "Bearer 你的令牌" }
    }
  }
}
```

客户端**必须**带 `Authorization: Bearer <令牌>`：
配了 `mcp.token` 就只认它，否则认 `authToken`；都没配时只允许本机访问
（跨设备连会拿到 401/403，容易被误判成"连不上"）。

协议行为：`POST /mcp` 无状态 JSON-RPC；`initialize` 回显客户端 `protocolVersion`；
通知返回 202；`GET` 返回 405 让客户端降级；`DELETE` 200 结束会话；CORS 放开。

## 接口

```http
POST /v1/agent/message
Authorization: Bearer <authToken>
{"text":"今天运行了哪些任务？","sessionId":"default"}

→ {"sessionId":"default","events":[{"type":"message","text":"..."}]}
```

```http
POST /v1/agent/confirm
{"action":{"id":"act_...","toolName":"run_task","arguments":{"id":1103},"summary":"运行任务 1103"},"approved":true}
```

事件类型：`message` / `candidates` / `needConfirmation` / `error`。

健康检查：`GET /health` → `{"ok":true,"version":"...","timezone":"..."}`（固定免鉴权，
安装脚本的就绪检查、systemd/容器探针、反代健康检查都裸打它）。

## 反向代理 / HTTPS

令牌是明文 Bearer，跨机器访问必须走 HTTPS。服务对反代是透明的：按
`X-Forwarded-Proto` / `X-Forwarded-Host` 推导对外地址，不需要额外开关。
MCP 是单次 JSON-RPC（不开 SSE），QQ bot 是出站连接——**反代不用配 WebSocket / SSE**。

| 端口 | 内容 | 建议 |
|---|---|---|
| 18790 | `/health`、`/v1/agent/*`、`/mcp` | 暴露给客户端 |
| 18791 | 管理页 + `/api/*` | 能不上公网就不上（SSH 隧道 / 内网白名单） |

单域名按路径分流即可。安装时就把两个端口收回环，并填好对外地址：

```bash
curl -fsSL https://raw.githubusercontent.com/zhengsh2822/qinglong-ai-services/main/deploy/install.sh \
  | sudo bash -s -- \
      --token 全局令牌 \
      --bind 127.0.0.1 \
      --webui-bind 127.0.0.1 \
      --public-url https://ai.example.com
```

> 反代不在 443 标准端口时（如 `https://host:8443`），**必须**填 `--public-url`
> 或在管理页填 `webui.publicUrl`。否则按 Host 端口推导会把 MCP 端点落到 18790，客户端连错。

### Caddy

公网域名（80/443 可达时自动签 Let's Encrypt）：

```caddyfile
ai.example.com {
	handle /api/*  { reverse_proxy 127.0.0.1:18791 }
	handle /health { reverse_proxy 127.0.0.1:18790 }
	handle /v1/*   { reverse_proxy 127.0.0.1:18790 }
	handle /mcp    { reverse_proxy 127.0.0.1:18790 }
	handle         { reverse_proxy 127.0.0.1:18791 }
}
```

局域网自签（域名或 IP 都行）：

```caddyfile
{
	# 不往系统信任库装根证书，改由你分发 root.crt
	skip_install_trust
}

https://192.168.1.10 {
	tls internal

	handle /api/*  { reverse_proxy 127.0.0.1:18791 }
	handle /health { reverse_proxy 127.0.0.1:18790 }
	handle /v1/*   { reverse_proxy 127.0.0.1:18790 }
	handle /mcp    { reverse_proxy 127.0.0.1:18790 }
	handle         { reverse_proxy 127.0.0.1:18791 }
}
```

根证书位置：Linux `/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt`，
Windows `%APPDATA%\Caddy\pki\authorities\local\root.crt`。装到客户端信任库才无警告。

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name ai.example.com;

    ssl_certificate     /etc/letsencrypt/live/ai.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/ai.example.com/privkey.pem;

    client_max_body_size 1m;
    proxy_http_version 1.1;
    proxy_set_header Host              $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host  $host;
    proxy_set_header X-Real-IP         $remote_addr;

    location = /health { proxy_pass http://127.0.0.1:18790; }
    location /mcp      { proxy_pass http://127.0.0.1:18790; }

    location /v1/ {
        proxy_pass http://127.0.0.1:18790;
        # 同步等模型回答，默认 60s 会被掐断
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }

    location /api/ { proxy_pass http://127.0.0.1:18791; }
    location /     { proxy_pass http://127.0.0.1:18791; }
}

server {
    listen 80;
    server_name ai.example.com;
    return 301 https://$host$request_uri;
}
```

局域网没域名用自签，客户端导入 `ai.crt`：

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout ai.key -out ai.crt -subj "/CN=ai.lan" \
  -addext "subjectAltName=DNS:ai.lan,IP:192.168.1.10"
```

### 加固与自检

- 三个令牌分开：`authToken`（API）、`mcp.token`（给 Cursor / Claude Code）、`webui.token`（管理页）。
- 管理页不想公开：`location /` 加 `allow 192.168.0.0/16; deny all;`，或只走 SSH 隧道。
- 防刷模型额度：给 `/v1/agent/message` 加 `limit_req`。

```bash
curl -fsS https://ai.example.com/health                                      # 200
curl -s -o /dev/null -w '%{http_code}\n' https://ai.example.com/api/status    # 401
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer 全局令牌" \
     https://ai.example.com/api/status                                        # 200
curl -s -X POST https://ai.example.com/mcp \
  -H "Authorization: Bearer MCP令牌" -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}'
```

客户端填的 MCP 地址就是 `https://ai.example.com/mcp` +
`Authorization: Bearer <mcp.token>`。

## 安全

- 默认只监听回环；对外必须设 `authToken`，否则非本机请求直接 403。
- `/health` 固定免鉴权（只回 ok/版本/时区），其余接口和 `/mcp` 都要令牌。
- 管理页 `/api/*` 需要 `authToken` 或 `webui.token`；页面本身是静态 HTML，不含密钥。
- `run_task` / `stop_task` 永远需要人工确认。
- 工具结果经脱敏规则处理（`value` / `token` / `cookie` 等字段打码）后才交给模型。
- 审计：`audit.jsonl`（谁、何时、调了什么、是否确认）。
- 管理页只经 HTTP、别直接暴露到公网裸端口；要么 SSH 隧道，要么反代加 HTTPS（见「反向代理 / HTTPS」）。

## 构建与开发

```bash
./build.sh
# KSU 模块 : ../apk_output/ksu_agent_core_v<version>.zip
# 服务器版 : dist/agentcore-linux-amd64
# 服务器版 : dist/agentcore-linux-arm64
```

构建会先把 `webroot/index.html` 同步到 `src/internal/webui/`（内嵌进二进制），
所以改了页面记得跑一次 `build.sh`；服务器上也可以只替换 `<prefix>/webroot/index.html`
让管理页立即更新，不必重编。

测试：

```bash
cd src && go test ./...
```

## 目录

```
.
├── module.prop              # KSU 模块元数据（id 保持 ksu_agent_core，升级用）
├── customize.sh             # KSU 安装：权限 + 数据目录
├── run.sh / service.sh      # KSU 启动与 watchdog 守护
├── boot-completed.sh        # 开机完成再确认
├── uninstall.sh             # KSU 卸载：只停进程，保留配置审计
├── default-config.json      # KSU 首次启动的配置模板
├── src/                     # Go 源码（main / agent / mcp / webui / qinglong / qqbot）
├── webroot/index.html       # 唯一的管理页源文件（KSU 与服务器共用）
├── deploy/
│   ├── install.sh           # 服务器一键安装
│   ├── uninstall.sh         # 服务器卸载
│   └── docker-entrypoint.sh # 容器入口（首启生成配置）
├── tools/deploy.sh          # KSU 热更新（推二进制 + 页面并重启）
├── Dockerfile
├── docker-compose.yml
└── build.sh
```

## 与 App 对接

App →「我的 → 高级功能 → AI 助手 → 设置」：

- 后端档位：**远程服务**
- 服务地址：`http://127.0.0.1:18790`（同机走回环）或服务器地址
- 访问令牌：与 `authToken` 一致

App 按同一套事件协议渲染消息、候选与确认卡片，写操作必须确认后才执行。
