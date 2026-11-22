package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSanitizeMasksSecrets(t *testing.T) {
	out := sanitize(`{"value":"abc","token":"xyz","name":"ok"}`, 1000)
	if strings.Contains(out, `"abc"`) || strings.Contains(out, `"xyz"`) {
		t.Fatalf("secrets not masked: %s", out)
	}
	if !strings.Contains(out, `"name":"ok"`) {
		t.Fatalf("normal field lost: %s", out)
	}
}

func TestParseCandidates(t *testing.T) {
	raw := `{"code":200,"data":{"data":[{"id":1103,"name":"品赞自动IP白名单","status":1,"schedule":"35 0-23/1 * * *"}]}}`
	cs := parseCandidates(raw)
	if len(cs) != 1 || cs[0].ID != "1103" || cs[0].Title != "品赞自动IP白名单" {
		t.Fatalf("unexpected candidates: %+v", cs)
	}
	if !strings.Contains(cs[0].Subtitle, "35 0-23/1") {
		t.Fatalf("schedule lost: %+v", cs[0])
	}
}

func TestWithAuthLoopbackOnly(t *testing.T) {
	h := withAuth("", "", "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/agent/message", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback should pass, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/agent/message", nil)
	req2.RemoteAddr = "10.0.0.8:1234"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("non-loopback should be 403, got %d", rec2.Code)
	}
}

// /health 固定免鉴权：安装脚本的就绪检查与各类探针都是裸打它，
// 不能因为配了令牌就 401，否则服务正常也会被判成没起来。
func TestHealthBypassesAuth(t *testing.T) {
	h := withAuth("s3cret", "", "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "10.0.0.8:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health should be public, got %d", rec.Code)
	}
}

func TestWithAuthToken(t *testing.T) {
	h := withAuth("s3cret", "", "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ok := httptest.NewRequest(http.MethodGet, "/v1/agent/message", nil)
	ok.RemoteAddr = "10.0.0.8:1234"
	ok.Header.Set("Authorization", "Bearer s3cret")
	recOK := httptest.NewRecorder()
	h.ServeHTTP(recOK, ok)
	if recOK.Code != http.StatusOK {
		t.Fatalf("valid token should pass, got %d", recOK.Code)
	}

	bad := httptest.NewRequest(http.MethodGet, "/v1/agent/message", nil)
	bad.RemoteAddr = "10.0.0.8:1234"
	bad.Header.Set("Authorization", "Bearer wrong")
	recBad := httptest.NewRecorder()
	h.ServeHTTP(recBad, bad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token should be 401, got %d", recBad.Code)
	}
}

func TestToolDefsUniqueAndComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range toolDefs() {
		name := def.Function.Name
		if seen[name] {
			t.Fatalf("duplicate tool: %s", name)
		}
		seen[name] = true
		if def.Function.Description == "" {
			t.Fatalf("tool %s missing description", name)
		}
	}
	for _, required := range []string{
		"server_status", "list_tasks", "today_runs", "today_successes",
		"today_failures", "runtime_status", "task_log", "run_task", "stop_task",
	} {
		if !seen[required] {
			t.Fatalf("missing tool: %s", required)
		}
	}
}

func TestRequiresConfirmation(t *testing.T) {
	if !requiresConfirmation("run_task") || !requiresConfirmation("stop_task") {
		t.Fatal("write tools must require confirmation")
	}
	if requiresConfirmation("list_tasks") || requiresConfirmation("task_log") {
		t.Fatal("read-only tools must not require confirmation")
	}
}

// MCP 独立令牌：配了 mcp.token 后该路径只认它，全局 authToken 不通用。
func TestWithAuthMCPToken(t *testing.T) {
	h := withAuth("global", "/mcp", "mcp-secret",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	mcpOK := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	mcpOK.RemoteAddr = "10.0.0.8:1234"
	mcpOK.Header.Set("Authorization", "Bearer mcp-secret")
	recOK := httptest.NewRecorder()
	h.ServeHTTP(recOK, mcpOK)
	if recOK.Code != http.StatusOK {
		t.Fatalf("mcp token should pass, got %d", recOK.Code)
	}

	mcpBad := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	mcpBad.RemoteAddr = "10.0.0.8:1234"
	mcpBad.Header.Set("Authorization", "Bearer global")
	recBad := httptest.NewRecorder()
	h.ServeHTTP(recBad, mcpBad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("global token should not pass mcp path, got %d", recBad.Code)
	}

	other := httptest.NewRequest(http.MethodGet, "/v1/agent/message", nil)
	other.RemoteAddr = "10.0.0.8:1234"
	other.Header.Set("Authorization", "Bearer global")
	recOther := httptest.NewRecorder()
	h.ServeHTTP(recOther, other)
	if recOther.Code != http.StatusOK {
		t.Fatalf("global token should pass other paths, got %d", recOther.Code)
	}
}
