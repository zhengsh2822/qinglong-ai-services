# Qinglong AI Services —— 单镜像包含 API、MCP 与管理页
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/agentcore .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 qas

COPY --from=build /out/agentcore /usr/local/bin/agentcore
COPY webroot/index.html /opt/qinglong/webroot/index.html
COPY deploy/docker-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/agentcore /usr/local/bin/entrypoint.sh

VOLUME ["/data"]
EXPOSE 18790 18791

# 管理页要读写 /data（配置、审计、日志），所以不切换到非 root 用户；
# 想降权就把 /data 的属主改成 qas 再取消下面这行的注释
# USER qas

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
