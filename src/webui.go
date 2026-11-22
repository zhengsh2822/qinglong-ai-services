package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// webuiIndex 是管理页的内嵌副本，构建时由 build.sh 从 webroot/index.html 同步。
//
// KSU 模块用管理器自带的 WebUI（ksu.exec 直接跑 root 命令），那份 webroot/index.html
// 由管理器读取；云服务器没有 ksu.exec，同一个页面改走 /api/*，所以要把 HTML 打进二进制，
// 单文件即可部署。运行期若存在 <exe>/webroot/index.html 则优先用磁盘文件，
// 这样服务器上更新界面不必重编二进制。
//
//go:embed internal/webui/index.html
var webuiIndex []byte

const (
	webuiDefaultListen = "127.0.0.1:18791"
	maxAdminBody       = 1 << 20 // 1MB，配置不会这么大
)

type webUIServer struct {
	cfgPath   string
	auditPath string
	logPath   string
	webRoot   string
	version   string
	started   time.Time
	cfg       *Config
}

func newWebUIServer(cfg *Config, cfgPath, auditPath, logPath, webRoot, version string) *webUIServer {
	return &webUIServer{
		cfgPath:   cfgPath,
		auditPath: auditPath,
		logPath:   logPath,
		webRoot:   webRoot,
		version:   version,
		started:   time.Now(),
		cfg:       cfg,
	}
}

func (s *webUIServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/config", s.auth(s.handleConfig))
	mux.HandleFunc("/api/logs", s.auth(s.handleLogs))
	mux.HandleFunc("/api/audit", s.auth(s.handleAudit))
	mux.HandleFunc("/api/restart", s.auth(s.handleRestart))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// ── 鉴权 ────────────────────────────────────────────────────────────

func (s *webUIServer) adminToken() string {
	if s.cfg == nil {
		return ""
	}
	return s.cfg.WebUI.TokenOr(s.cfg.AuthToken)
}

// auth 只保护 /api/*：页面本身是静态 HTML，不含密钥，允许匿名打开；
// 没配令牌时退化为「仅本机」，与主服务的规则保持一致。
func (s *webUIServer) auth(next http.HandlerFunc) http.HandlerFunc {
	token := s.adminToken()
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if got == "" {
				got = r.Header.Get("X-Admin-Token")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error": "UNAUTHORIZED", "authRequired": true,
				})
				return
			}
		} else {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || (host != "127.0.0.1" && host != "::1") {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error": "未配置访问令牌时只接受本机访问", "authRequired": true,
				})
				return
			}
		}
		next(w, r)
	}
}

// ── 静态页 ──────────────────────────────────────────────────────────

func (s *webUIServer) indexHTML() []byte {
	if s.webRoot != "" {
		if b, err := os.ReadFile(filepath.Join(s.webRoot, "index.html")); err == nil && len(b) > 0 {
			return b
		}
	}
	return webuiIndex
}

func (s *webUIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.indexHTML())
}

// ── 状态 ────────────────────────────────────────────────────────────

func (s *webUIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	mcpEnabled := cfg != nil && cfg.MCP.IsEnabled()
	mcpPath, endpoint := "", ""
	if mcpEnabled {
		mcpPath = cfg.MCP.NormalizePath()
		endpoint = s.publicBase(r) + mcpPath
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"mode":     "server",
		"version":  s.version,
		"pid":      os.Getpid(),
		"uptime":   int64(time.Since(s.started).Seconds()),
		"now":      time.Now().Format("2006-01-02 15:04:05 MST"),
		"timezone": time.Local.String(),
		"listen":   cfg.Listen,
		"webui": map[string]any{
			"listen":       cfg.WebUI.NormalizeListen(),
			"authRequired": s.adminToken() != "",
		},
		"mcp": map[string]any{
			"enabled": mcpEnabled, "path": mcpPath, "endpoint": endpoint,
		},
		"qqbot":    map[string]any{"enabled": cfg.QQBot.Enabled && cfg.QQBot.AppID != ""},
		"qinglong": map[string]any{"baseUrl": cfg.QingLong.BaseURL},
		"paths": map[string]any{
			"config": s.cfgPath, "log": s.logPath, "audit": s.auditPath,
		},
	})
}

// publicBase 推导对外基础地址，用来拼 MCP 端点给客户端填。
//
// 优先级：webui.publicUrl（反代场景必须配） → 请求 Host 启发式。
// 启发式规则：Host 自带端口说明是直连管理端口（如 1.2.3.4:18791），
// 换成主服务端口；Host 没有端口说明走的是 80/443 或反代，保持原样。
func (s *webUIServer) publicBase(r *http.Request) string {
	if s.cfg != nil {
		if u := strings.TrimRight(strings.TrimSpace(s.cfg.WebUI.PublicURL), "/"); u != "" {
			return u
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := headerFirst(r, "X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	host := headerFirst(r, "X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	// Host 带端口说明访问的是管理端口本身（如 1.2.3.4:18791），
	// 客户端要连的是主服务端口，这里换掉；Host 没端口说明走 80/443 或反代，保持原样。
	if h, _, err := net.SplitHostPort(host); err == nil && s.cfg != nil {
		if _, mainPort, perr := net.SplitHostPort(s.cfg.Listen); perr == nil && mainPort != "" && !defaultPort(scheme, mainPort) {
			host = net.JoinHostPort(h, mainPort)
		}
	}
	return scheme + "://" + host
}

func headerFirst(r *http.Request, key string) string {
	v := r.Header.Get(key)
	if v == "" {
		return ""
	}
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func defaultPort(scheme, port string) bool {
	return (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
}

// ── 配置读写 ────────────────────────────────────────────────────────

func (s *webUIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(s.cfgPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "读取配置失败：" + err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	case http.MethodPut, http.MethodPost:
		s.updateConfig(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "only GET/PUT"})
	}
}

func (s *webUIServer) updateConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAdminBody))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "读取请求体失败"})
		return
	}
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体不是合法 JSON 对象"})
		return
	}
	base := map[string]any{}
	if old, err := os.ReadFile(s.cfgPath); err == nil {
		_ = json.Unmarshal(old, &base)
	}
	merged := mergeConfigMap(base, patch)
	// 页面不编辑的字段（skill / dns / protocol…）靠合并保留，不会被整文件覆盖掉。
	// 但 webui 段必须留在服务端：页面即使没提交，也不能因为合并范围漏掉它。
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "序列化失败：" + err.Error()})
		return
	}
	if err := writeFileAtomic(s.cfgPath, append(out, '\n'), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "写入配置失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restarting": true})
	s.scheduleRestart("配置已更新")
}

// mergeConfigMap 逐层浅合并：顶层键直接用新值覆盖，子对象再合并一层。
// 页面只提交它认识的字段，其它键（skill/dns/未知扩展）必须原样保留。
func mergeConfigMap(base, patch map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		if bm, ok := out[k].(map[string]any); ok {
			if pm, ok2 := v.(map[string]any); ok2 {
				merged := make(map[string]any, len(bm)+len(pm))
				for bk, bv := range bm {
					merged[bk] = bv
				}
				for pk, pv := range pm {
					merged[pk] = pv
				}
				out[k] = merged
				continue
			}
		}
		out[k] = v
	}
	return out
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ── 日志 / 审计 / 重启 ──────────────────────────────────────────────

func (s *webUIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	s.tail(w, s.logPath, r)
}

func (s *webUIServer) handleAudit(w http.ResponseWriter, r *http.Request) {
	s.tail(w, s.auditPath, r)
}

func (s *webUIServer) tail(w http.ResponseWriter, path string, r *http.Request) {
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "", "note": "未配置日志路径"})
		return
	}
	n := 80
	if v := r.URL.Query().Get("lines"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 500 {
		n = 500
	}
	text, err := tailFile(path, n)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "text": "", "note": "暂无日志：" + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": text, "path": path})
}

// tailFile 从文件末尾读一块再切行，避免日志涨到几十 MB 后整文件读进内存。
func tailFile(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	if size == 0 {
		return "", nil
	}
	const chunk = 64 << 10
	start := int64(0)
	if size > chunk {
		start = size - chunk
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	buf := make([]byte, size-start)
	read, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	buf = buf[:read]
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if start > 0 && len(lines) > 1 {
		lines = lines[1:] // 首行可能是被截断的半行
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}

func (s *webUIServer) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "only POST"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restarting": true})
	s.scheduleRestart("收到重启请求")
}

// scheduleRestart 让进程主动退出，由守护方拉起：systemd 用 Restart=always，
// KSU 用 watchdog 循环。这样不必在进程内做 exec 自我替换，两种环境同一套逻辑。
func (s *webUIServer) scheduleRestart(reason string) {
	log.Printf("webui: %s，进程即将退出等待守护重启", reason)
	go func() {
		time.Sleep(400 * time.Millisecond)
		os.Exit(0)
	}()
}

// webuiHint 把监听地址转成能直接点的 URL，0.0.0.0 / :: 回落本机。
func webuiHint(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
