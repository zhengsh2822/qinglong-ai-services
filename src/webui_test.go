package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMergeConfigMapPreservesUnknownFields(t *testing.T) {
	base := map[string]any{
		"listen": "0.0.0.0:18790",
		"skill":  map[string]any{"enabled": true, "path": "/opt/openapi.md"},
		"llm":    map[string]any{"baseUrl": "https://a/v1", "protocol": "anthropic"},
	}
	patch := map[string]any{
		"llm":       map[string]any{"baseUrl": "https://b/v1"},
		"maxRounds": float64(3),
	}
	out := mergeConfigMap(base, patch)

	llm, _ := out["llm"].(map[string]any)
	if llm["baseUrl"] != "https://b/v1" {
		t.Fatalf("patch 未覆盖 llm.baseUrl：%v", llm)
	}
	if llm["protocol"] != "anthropic" {
		t.Fatalf("页面没提交的子字段被丢了：%v", llm)
	}
	if _, ok := out["skill"]; !ok {
		t.Fatalf("页面不编辑的 skill 段被丢了")
	}
	if out["maxRounds"] != float64(3) {
		t.Fatalf("maxRounds 未更新：%v", out["maxRounds"])
	}
}

func TestTailFileKeepsLastLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout.log")
	var sb strings.Builder
	for i := 1; i <= 200; i++ {
		sb.WriteString("line-")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := tailFile(path, 3)
	if err != nil {
		t.Fatalf("tailFile 失败：%v", err)
	}
	want := "line-198\nline-199\nline-200"
	if got != want {
		t.Fatalf("tail 结果不对，want %q got %q", want, got)
	}

	empty := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := tailFile(empty, 5); err != nil || got != "" {
		t.Fatalf("空文件应返回空串，got %q err %v", got, err)
	}
}

func TestWebUIAuthToken(t *testing.T) {
	cfg := &Config{AuthToken: "global", WebUI: WebUIConfig{Token: "admin"}}
	srv := newWebUIServer(cfg, "", "", "", "", "test")
	h := srv.handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401，got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer global")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("webui.token 配了之后不该认全局令牌，got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("X-Admin-Token", "admin")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确令牌应 200，got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"mode":"server"`) {
		t.Fatalf("status 缺 mode 字段：%s", rec.Body.String())
	}
}

func TestWebUINoTokenOnlyLoopback(t *testing.T) {
	srv := newWebUIServer(&Config{}, "", "", "", "", "test")
	h := srv.handler()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "196.1.2.3:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("未配令牌时外部地址应 403，got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("本机访问应 200，got %d", rec.Code)
	}
}

func TestPublicBaseHeuristics(t *testing.T) {
	cfg := &Config{Listen: "0.0.0.0:18790"}
	srv := newWebUIServer(cfg, "", "", "", "", "test")

	// 直连管理端口：Host 带端口 → 换成主服务端口
	req := httptest.NewRequest(http.MethodGet, "http://1.2.3.4:18791/api/status", nil)
	if got := srv.publicBase(req); got != "http://1.2.3.4:18790" {
		t.Fatalf("直连推导错误：%s", got)
	}

	// 反代：Host 无端口 → 保持 scheme://host
	req = httptest.NewRequest(http.MethodGet, "https://ai.example.com/api/status", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := srv.publicBase(req); got != "https://ai.example.com" {
		t.Fatalf("反代推导错误：%s", got)
	}

	// 配了 publicUrl 就完全听它的
	cfg.WebUI.PublicURL = "https://ai.example.com/qas/"
	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18791/api/status", nil)
	if got := srv.publicBase(req); got != "https://ai.example.com/qas" {
		t.Fatalf("publicUrl 未生效：%s", got)
	}
}
