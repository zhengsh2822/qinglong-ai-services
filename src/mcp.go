package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
)

// MCP Server：把青龙工具按 Model Context Protocol 暴露出去，
// 让 Cursor / Claude Code / Cherry Studio 等 MCP 客户端直接调用青龙能力。
//
// 传输：Streamable HTTP（POST /mcp 单次 JSON-RPC 请求/响应，不开 SSE 长连接）。
// 方法：initialize / ping / tools/list / tools/call；
// resources/list、prompts/list 返回空列表，避免客户端探测时判定服务端异常。
//
// 兼容行为与 App 端 agent_mcp_server.dart 保持一致：
// 版本回显、通知 202、GET 405 降级、DELETE 200、CORS 放开。
type MCPHandler struct {
	agent *Agent
}

func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers",
		"Content-Type, Accept, Mcp-Session-Id, MCP-Protocol-Version")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")

	switch r.Method {
	case http.MethodOptions:
		// CORS 预检
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodDelete:
		// 会话终止：本服务无状态，直接确认
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodGet:
		// 客户端探测 SSE 长连接：本服务只做请求-响应，回 405 让其降级为 POST
		w.Header().Set("Allow", "POST, OPTIONS, DELETE")
		http.Error(w, "SSE stream not supported, use POST", http.StatusMethodNotAllowed)
		return
	case http.MethodPost:
		// 继续处理
	default:
		w.Header().Set("Allow", "POST, OPTIONS, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeRPCError(w, nil, -32700, "parse error")
		return
	}

	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		writeRPCError(w, nil, -32700, "invalid json")
		return
	}

	// 通知（notifications/initialized 等）无 id：按规范回 202 空体。
	// 回 JSON-RPC 响应会让严格客户端判定握手失败。
	if len(req.ID) == 0 || strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var result any
	switch req.Method {
	case "initialize":
		result = mcpInitialize(req.Params)
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": mcpToolDefs()}
	case "tools/call":
		result = h.mcpToolCall(req.Params)
	case "resources/list":
		result = map[string]any{"resources": []any{}}
	case "resources/templates/list":
		result = map[string]any{"resourceTemplates": []any{}}
	case "prompts/list":
		result = map[string]any{"prompts": []any{}}
	case "logging/setLevel":
		result = map[string]any{}
	default:
		writeRPCError(w, req.ID, -32601, "method not found: "+req.Method)
		return
	}

	writeRPCResult(w, req.ID, result)
}

// mcpInitialize 回显客户端请求的协议版本。
// 固定写死某一版会让另一侧的客户端直接判定版本不兼容、中止握手。
func mcpInitialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	pv := p.ProtocolVersion
	if pv == "" {
		pv = "2025-06-18"
	}
	return map[string]any{
		"protocolVersion": pv,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo": map[string]any{
			"name":    "qinglong-agent",
			"version": "1.0.0",
		},
		"instructions": "青龙面板工具集。调用 tools/list 查看可用能力。",
	}
}

func (h *MCPHandler) mcpToolCall(params json.RawMessage) any {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	if h.agent == nil || h.agent.ql == nil {
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "服务未就绪"}},
			"isError": true,
		}
	}
	text, err := h.agent.ql.executeTool(p.Name, p.Arguments)
	isErr := false
	if err != nil {
		text = "执行失败：" + err.Error()
		isErr = true
	}
	res := map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
	}
	if isErr {
		res["isError"] = true
	}
	return res
}

// mcpToolDefs 把 Agent 的工具定义映射为 MCP 的 inputSchema 格式。
func mcpToolDefs() []map[string]any {
	defs := toolDefs()
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]any{
			"name":        d.Function.Name,
			"description": d.Function.Description,
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": d.Function.Parameters["properties"],
				"required":   d.Function.Parameters["required"],
			},
		})
	}
	return out
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	if len(id) == 0 {
		id = json.RawMessage("0")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage("0")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	})
}

// MCP 端点说明（给客户端配置用）。
// 监听在 0.0.0.0 / :: / 空 host 时，提示地址回落到 127.0.0.1，避免拼出
// "http://127.0.0.10.0.0.0:18790/mcp" 这类错误 URL。
func mcpEndpointHint(listen, path string) string {
	if path == "" {
		path = "/mcp"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://127.0.0.1:18790" + path
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + path
}
