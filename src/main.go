package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // 内嵌时区库：Android 的 zoneinfo 路径不被 Go 默认识别
)

// serviceVersion 由 build.sh 用 -ldflags -X 从 module.prop 注入，
// 保证日志版本与 KSU 管理器显示的模块版本一致。
var serviceVersion = "dev"

func main() {
	dataDir := defaultDataDir()
	configPath := flag.String("config", filepath.Join(dataDir, "config.json"), "配置文件路径")
	auditPath := flag.String("audit", filepath.Join(dataDir, "audit.jsonl"), "审计日志路径")
	logPath := flag.String("log", "", "运行日志路径（缺省用审计文件同目录的 stdout.log）")
	webRoot := flag.String("webroot", defaultWebRoot(), "WebUI 静态目录（缺省用内嵌副本）")
	flag.Parse()

	effectiveLog := strings.TrimSpace(*logPath)
	if effectiveLog == "" {
		effectiveLog = filepath.Join(filepath.Dir(*auditPath), "stdout.log")
	}

	initTimezone()
	now := time.Now()
	log.Printf("agentcore %s | timezone=%s | now=%s",
		serviceVersion, time.Local.String(), now.Format("2006-01-02 15:04:05 MST"))

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}
	initResolver(cfg.DNS)
	agent := newAgent(cfg, *auditPath)

	if cfg.QQBot.Enabled && cfg.QQBot.AppID != "" && cfg.QQBot.AppSecret != "" {
		go newQQBot(cfg.QQBot, agent).Run(context.Background())
		log.Printf("qqbot: 已启用（appId=%s）", cfg.QQBot.AppID)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "service": "agentcore", "version": serviceVersion,
			"timezone": time.Local.String(),
			"now":      time.Now().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/v1/agent/message", agent.handleMessageHTTP)
	mux.HandleFunc("/v1/agent/confirm", agent.handleConfirmHTTP)
	mcpPath := ""
	if cfg.MCP.IsEnabled() {
		mcpPath = cfg.MCP.NormalizePath()
		mux.Handle(mcpPath, &MCPHandler{agent: agent})
		log.Printf("MCP endpoint: %s", mcpEndpointHint(cfg.Listen, mcpPath))
	}

	// 云端/服务器模式：进程自带管理页。KSU 模块下由管理器提供 WebUI，
	// 这个开关默认关闭，打开也只是多监听一个端口。
	if cfg.WebUI.IsEnabled() {
		startWebUI(cfg, *configPath, *auditPath, effectiveLog, *webRoot)
	}

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           withAuth(cfg.AuthToken, mcpPath, cfg.MCP.Token, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("agentcore listening on %s", cfg.Listen)
	log.Fatal(server.ListenAndServe())
}

// defaultDataDir 兼顾两种部署形态：KSU 模块固定 /data/adb/agentcore，
// 服务器上则落在二进制同目录，方便 install.sh 把整个服务装进一个路径。
func defaultDataDir() string {
	if st, err := os.Stat("/data/adb/agentcore"); err == nil && st.IsDir() {
		return "/data/adb/agentcore"
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// defaultWebRoot 优先用磁盘上的 webroot（服务器上换界面不必重编二进制），
// 找不到就返回空串，由内嵌副本兜底。
func defaultWebRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	base := filepath.Dir(exe)
	for _, dir := range []string{
		filepath.Join(base, "webroot"),
		filepath.Join(base, "..", "webroot"),
	} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}

func startWebUI(cfg *Config, cfgPath, auditPath, logPath, webRoot string) {
	addr := cfg.WebUI.NormalizeListen()
	if addr == cfg.Listen {
		log.Printf("webui: 监听地址与主服务重复（%s），已跳过", addr)
		return
	}
	srv := newWebUIServer(cfg, cfgPath, auditPath, logPath, webRoot, serviceVersion)
	if isWildcardHost(addr) && srv.adminToken() == "" {
		log.Printf("webui: 监听 %s 但没配令牌，非本机访问会被拒绝；先设 authToken 或 webui.token", addr)
	}
	go func() {
		log.Printf("webui: %s", webuiHint(addr))
		if err := http.ListenAndServe(addr, srv.handler()); err != nil {
			log.Printf("webui 启动失败：%v", err)
		}
	}()
}

func isWildcardHost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return host == "" || host == "0.0.0.0" || host == "::"
}

// initTimezone 修正 init 服务环境没有 TZ 的问题。
// 优先级：TZ 环境变量 → 设备系统属性 persist.sys.timezone。
// tzdata 已内嵌，LoadLocation 在 Android 上也能解析 Asia/Shanghai。
func initTimezone() {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			time.Local = loc
			return
		}
	}
	for _, bin := range []string{"/system/bin/getprop", "getprop"} {
		out, err := exec.Command(bin, "persist.sys.timezone").Output()
		if err != nil {
			continue
		}
		tz := strings.TrimSpace(string(out))
		if tz == "" {
			continue
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			continue
		}
		time.Local = loc
		_ = os.Setenv("TZ", tz)
		return
	}
}

// withAuth：配置了 authToken 就强制 Bearer；没配置则只接受本机回环访问。
// withAuth 统一鉴权。
//
// MCP 端点可以配独立令牌（mcp.token）：配了之后该路径只认这个令牌，
// 方便把 MCP 单独发给 Cursor / Claude Code 而不暴露全局 authToken。
// 未配独立令牌时沿用全局规则（authToken 为空则只允许本机）。
func withAuth(token, mcpPath, mcpToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mcpPath != "" && r.URL.Path == mcpPath && mcpToken != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(mcpToken)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "UNAUTHORIZED"})
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if token == "" {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || (host != "127.0.0.1" && host != "::1") {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error": "未配置 authToken 时只接受本机访问",
				})
				return
			}
		} else {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "UNAUTHORIZED"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Agent) handleMessageHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "only POST"})
		return
	}
	var req struct {
		Text      string `json:"text"`
		SessionID string `json:"sessionId"`
		UseSkill  *bool  `json:"useSkill,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体不是合法 JSON"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text 不能为空"})
		return
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = "default"
	}
	events := a.HandleMessage(sessionID, req.Text, req.UseSkill)
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sessionID,
		"events":    events,
	})
}

func (a *Agent) handleConfirmHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "only POST"})
		return
	}
	var req struct {
		Action    PendingAction `json:"action"`
		Approved  bool          `json:"approved"`
		SessionID string        `json:"sessionId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体不是合法 JSON"})
		return
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = "default"
	}
	events := a.HandleConfirm(sessionID, req.Action, req.Approved)
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sessionID,
		"events":    events,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
