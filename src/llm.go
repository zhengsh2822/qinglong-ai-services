package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 模型 API 协议三选一（对齐主流客户端的「API 协议」选项）。
const (
	ProtoOpenAIChat     = "openai_chat"      // OpenAI Chat Completions（默认，兼容面最广）
	ProtoAnthropic      = "anthropic"        // Anthropic Messages
	ProtoOpenAIResponse = "openai_responses" // OpenAI Responses
)

type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type LLMClient struct {
	cfg  LLMConfig
	http *http.Client
}

func newLLMClient(cfg LLMConfig) *LLMClient {
	return &LLMClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *LLMClient) protocol() string {
	p := strings.ToLower(strings.TrimSpace(c.cfg.Protocol))
	switch p {
	case ProtoAnthropic, "anthropic_messages", "claude":
		return ProtoAnthropic
	case ProtoOpenAIResponse, "openai_response", "responses":
		return ProtoOpenAIResponse
	default:
		return ProtoOpenAIChat
	}
}

// chat 按协议分发。三种协议都在此归一为 OpenAI Chat 格式的 assistant message，
// 上层（extractToolCalls / 审计 / 事件）无需感知协议差异。
func (c *LLMClient) chat(messages []map[string]any, tools []ToolDef) (map[string]any, error) {
	switch c.protocol() {
	case ProtoAnthropic:
		return c.chatAnthropic(messages, tools)
	case ProtoOpenAIResponse:
		return c.chatResponses(messages, tools)
	default:
		return c.chatOpenAI(messages, tools)
	}
}

// ---------- OpenAI Chat Completions ----------

func (c *LLMClient) chatOpenAI(messages []map[string]any, tools []ToolDef) (map[string]any, error) {
	body := map[string]any{
		"model":    c.cfg.Model,
		"messages": messages,
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if c.cfg.Temperature != nil {
		body["temperature"] = *c.cfg.Temperature
	}

	parsed, err := c.postJSON(strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions",
		body, map[string]string{"Authorization": "Bearer " + c.cfg.APIKey})
	if err != nil {
		return nil, err
	}
	choices, _ := parsed["choices"].([]any)
	if len(choices) == 0 {
		return nil, fmt.Errorf("模型未返回 choices")
	}
	first, _ := choices[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	if message == nil {
		return nil, fmt.Errorf("模型返回缺少 message")
	}
	return message, nil
}

// ---------- Anthropic Messages ----------

// chatAnthropic 调 Anthropic /v1/messages，并把响应归一成 OpenAI 格式。
func (c *LLMClient) chatAnthropic(messages []map[string]any, tools []ToolDef) (map[string]any, error) {
	var system string
	var content []any

	flushToolResults := func(results []any) {
		if len(results) > 0 {
			content = append(content, map[string]any{"role": "user", "content": results})
		}
	}
	var pendingResults []any

	for _, m := range messages {
		role, _ := m["role"].(string)
		switch role {
		case "system":
			if s, ok := m["content"].(string); ok {
				system += s + "\n"
			}
		case "tool":
			// Anthropic 把 tool 结果放 user 消息的 tool_result block 里
			id, _ := m["tool_call_id"].(string)
			body, _ := m["content"].(string)
			pendingResults = append(pendingResults, map[string]any{
				"type":        "tool_result",
				"tool_use_id": id,
				"content":     body,
			})
		case "assistant":
			flushToolResults(pendingResults)
			pendingResults = nil
			blocks := []any{}
			if text, ok := m["content"].(string); ok && strings.TrimSpace(text) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
			if calls, ok := m["tool_calls"].([]any); ok {
				for _, raw := range calls {
					cm, _ := raw.(map[string]any)
					if cm == nil {
						continue
					}
					fn, _ := cm["function"].(map[string]any)
					if fn == nil {
						continue
					}
					name, _ := fn["name"].(string)
					argsRaw, _ := fn["arguments"].(string)
					var input map[string]any
					_ = json.Unmarshal([]byte(argsRaw), &input)
					if input == nil {
						input = map[string]any{}
					}
					id, _ := cm["id"].(string)
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    id,
						"name":  name,
						"input": input,
					})
				}
			}
			if len(blocks) > 0 {
				content = append(content, map[string]any{"role": "assistant", "content": blocks})
			}
		default: // user
			flushToolResults(pendingResults)
			pendingResults = nil
			if text, ok := m["content"].(string); ok {
				content = append(content, map[string]any{
					"role":    "user",
					"content": []any{map[string]any{"type": "text", "text": text}},
				})
			}
		}
	}
	flushToolResults(pendingResults)

	anthropicTools := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		anthropicTools = append(anthropicTools, map[string]any{
			"name":        t.Function.Name,
			"description": t.Function.Description,
			"input_schema": map[string]any{
				"type":       "object",
				"properties": t.Function.Parameters["properties"],
			},
		})
	}

	body := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": 4096,
		"messages":   content,
	}
	if strings.TrimSpace(system) != "" {
		body["system"] = strings.TrimSpace(system)
	}
	if len(anthropicTools) > 0 {
		body["tools"] = anthropicTools
	}
	if c.cfg.Temperature != nil {
		body["temperature"] = *c.cfg.Temperature
	}

	parsed, err := c.postJSON(strings.TrimRight(c.cfg.BaseURL, "/")+"/v1/messages",
		body, map[string]string{
			"x-api-key":         c.cfg.APIKey,
			"anthropic-version": "2023-06-01",
		})
	if err != nil {
		return nil, err
	}

	// 归一成 OpenAI assistant message
	out := map[string]any{"role": "assistant", "content": ""}
	var textParts []string
	var toolCalls []any
	if blocks, ok := parsed["content"].([]any); ok {
		for _, raw := range blocks {
			b, _ := raw.(map[string]any)
			if b == nil {
				continue
			}
			switch b["type"] {
			case "text":
				if t, ok := b["text"].(string); ok {
					textParts = append(textParts, t)
				}
			case "tool_use":
				id, _ := b["id"].(string)
				name, _ := b["name"].(string)
				input, _ := b["input"].(map[string]any)
				if input == nil {
					input = map[string]any{}
				}
				args, _ := json.Marshal(input)
				toolCalls = append(toolCalls, map[string]any{
					"id":   id,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": string(args),
					},
				})
			}
		}
	}
	out["content"] = strings.Join(textParts, "\n")
	if len(toolCalls) > 0 {
		out["tool_calls"] = toolCalls
	}
	return out, nil
}

// ---------- OpenAI Responses ----------

// chatResponses 调 OpenAI /v1/responses，并把响应归一成 Chat 格式。
func (c *LLMClient) chatResponses(messages []map[string]any, tools []ToolDef) (map[string]any, error) {
	var input []any
	for _, m := range messages {
		role, _ := m["role"].(string)
		switch role {
		case "tool":
			id, _ := m["tool_call_id"].(string)
			body, _ := m["content"].(string)
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": id,
				"output":  body,
			})
		case "assistant":
			item := map[string]any{"type": "message", "role": "assistant", "content": []any{
				map[string]any{"type": "output_text", "text": m["content"]},
			}}
			input = append(input, item)
			if calls, ok := m["tool_calls"].([]any); ok {
				for _, raw := range calls {
					cm, _ := raw.(map[string]any)
					if cm == nil {
						continue
					}
					fn, _ := cm["function"].(map[string]any)
					if fn == nil {
						continue
					}
					id, _ := cm["id"].(string)
					name, _ := fn["name"].(string)
					args, _ := fn["arguments"].(string)
					input = append(input, map[string]any{
						"type":      "function_call",
						"call_id":   id,
						"name":      name,
						"arguments": args,
					})
				}
			}
		default:
			input = append(input, map[string]any{
				"type":    "message",
				"role":    role,
				"content": m["content"],
			})
		}
	}

	respTools := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		respTools = append(respTools, map[string]any{
			"type":        "function",
			"name":        t.Function.Name,
			"description": t.Function.Description,
			"parameters":  t.Function.Parameters,
		})
	}

	body := map[string]any{
		"model": c.cfg.Model,
		"input": input,
	}
	if len(respTools) > 0 {
		body["tools"] = respTools
	}
	if c.cfg.Temperature != nil {
		body["temperature"] = *c.cfg.Temperature
	}

	parsed, err := c.postJSON(strings.TrimRight(c.cfg.BaseURL, "/")+"/v1/responses",
		body, map[string]string{"Authorization": "Bearer " + c.cfg.APIKey})
	if err != nil {
		return nil, err
	}

	out := map[string]any{"role": "assistant", "content": ""}
	var textParts []string
	var toolCalls []any
	if items, ok := parsed["output"].([]any); ok {
		for _, raw := range items {
			it, _ := raw.(map[string]any)
			if it == nil {
				continue
			}
			switch it["type"] {
			case "message":
				if arr, ok := it["content"].([]any); ok {
					for _, cr := range arr {
						cm, _ := cr.(map[string]any)
						if cm == nil {
							continue
						}
						if t, ok := cm["text"].(string); ok {
							textParts = append(textParts, t)
						}
					}
				}
			case "function_call":
				id, _ := it["call_id"].(string)
				name, _ := it["name"].(string)
				args, _ := it["arguments"].(string)
				toolCalls = append(toolCalls, map[string]any{
					"id":   id,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": args,
					},
				})
			}
		}
	}
	out["content"] = strings.Join(textParts, "\n")
	if len(toolCalls) > 0 {
		out["tool_calls"] = toolCalls
	}
	return out, nil
}

// ---------- 通用 HTTP ----------

func (c *LLMClient) postJSON(endpoint string, body map[string]any, headers map[string]string) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, apiErrorMessage(data))
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("模型返回格式异常：%v", err)
	}
	return parsed, nil
}

func apiErrorMessage(data []byte) string {
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err == nil {
		if e, ok := parsed["error"].(map[string]any); ok {
			if m, ok := e["message"].(string); ok && m != "" {
				return m
			}
		}
		if m, ok := parsed["message"].(string); ok && m != "" {
			return m
		}
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
