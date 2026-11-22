package main

import (
	"encoding/json"
	"os"
	"strings"
)

// LLMConfig 对应 App 里的「AI 设置 → 模型」，任意 OpenAI 兼容端点。
// Temperature 为 nil 时不传该字段（小米 MiMo 思考模式不支持自定义值）。
type LLMConfig struct {
	BaseURL     string   `json:"baseUrl"`
	APIKey      string   `json:"apiKey"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature,omitempty"`
	// Protocol 模型 API 协议三选一：openai_chat（默认）/ anthropic / openai_responses
	Protocol string `json:"protocol,omitempty"`
}

// QingLongConfig 青龙面板 OpenAPI。token 可用应用 token，
// 也可只填 clientId/clientSecret 由进程自动换取。
type QingLongConfig struct {
	BaseURL      string `json:"baseUrl"`
	Token        string `json:"token"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// QQBotConfig 对应 QQ 开放平台的机器人（WebSocket 模式，无需公网 IP）。
type QQBotConfig struct {
	Enabled   bool   `json:"enabled"`
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
	// Intents 默认 1<<25（GROUP_AND_C2C_EVENT，群 @ 与私聊消息）
	Intents int `json:"intents,omitempty"`
}

// SkillConfig 官方 Skill 路由手册（@whyour/qinglong-cli 的 skills/qinglong-cli）。
// 能力放服务端：Core 以 root 运行，读得到 proot 容器内的 openapi.md；
// 开关放客户端：enabled 可由请求覆盖，nil 表示默认启用。
type SkillConfig struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (s SkillConfig) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// MCPConfig 把青龙工具按 MCP 协议暴露给外部客户端（Cursor / Claude Code / Cherry Studio）。
//
// Core 常驻运行，App 退出后仍可 7×24 提供服务 —— 这是 MCP 的推荐接入位置；
// App 内的是临时端点，进程结束即消失。
type MCPConfig struct {
	Enabled *bool `json:"enabled,omitempty"`
	// Path 端点路径，默认 /mcp
	Path string `json:"path,omitempty"`
	// Token 独立访问令牌；空则沿用全局 authToken
	Token string `json:"token,omitempty"`
}

func (m MCPConfig) IsEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

// NormalizePath 归一化端点路径：保证以 / 开头、去掉尾部斜杠。
// ServeMux 按精确路径注册，用户填 "mcp" 或 "/mcp/" 都要能正确落到 /mcp。
func (m MCPConfig) NormalizePath() string {
	p := strings.TrimSpace(m.Path)
	if p == "" {
		return "/mcp"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

// WebUIConfig 管理页面。
//
// KSU 模块里页面由管理器注入 ksu.exec()，直接跑 root 命令，不需要进程自己起服务；
// 云服务器上没有 ksu.exec，改为进程内置一个静态页 + 管理 API（/api/*），
// 所以这个开关默认关闭，只在服务器安装时打开。
type WebUIConfig struct {
	Enabled *bool `json:"enabled,omitempty"`
	// Listen 管理页监听地址，默认 127.0.0.1:18791；要让公网/反代访问才改 0.0.0.0
	Listen string `json:"listen,omitempty"`
	// Token 管理页独立令牌；空则沿用全局 authToken
	Token string `json:"token,omitempty"`
	// PublicURL 对外访问地址（如 https://ai.example.com）。
	// 走反向代理时必填：否则 MCP 端点只能按请求 Host 猜，端口容易猜错。
	PublicURL string `json:"publicUrl,omitempty"`
}

func (w WebUIConfig) IsEnabled() bool {
	if w.Enabled == nil {
		return false
	}
	return *w.Enabled
}

func (w WebUIConfig) NormalizeListen() string {
	l := strings.TrimSpace(w.Listen)
	if l == "" {
		return webuiDefaultListen
	}
	return l
}

// TokenOr 管理页令牌：优先独立令牌，为空沿用全局访问令牌。
func (w WebUIConfig) TokenOr(global string) string {
	if t := strings.TrimSpace(w.Token); t != "" {
		return t
	}
	return global
}

type Config struct {
	Listen    string         `json:"listen"`
	AuthToken string         `json:"authToken"`
	LLM       LLMConfig      `json:"llm"`
	QingLong  QingLongConfig `json:"qinglong"`
	QQBot     QQBotConfig    `json:"qqbot"`
	Skill     SkillConfig    `json:"skill"`
	MCP       MCPConfig      `json:"mcp"`
	WebUI     WebUIConfig    `json:"webui,omitempty"`
	MaxRounds int            `json:"maxRounds"`
	// DNS 可选：留空时自动使用公共 DNS + 默认网关（Android 没有 resolv.conf）
	DNS string `json:"dns,omitempty"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:18790"
	}
	if cfg.MaxRounds <= 0 || cfg.MaxRounds > 8 {
		cfg.MaxRounds = 4
	}
	if cfg.Skill.Path == "" {
		cfg.Skill.Path = "/data/alpine/opt/ql-cli/lib/node_modules/@whyour/qinglong-cli/skills/qinglong-cli/references/openapi.md"
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.QingLong.BaseURL == "" {
		cfg.QingLong.BaseURL = "http://127.0.0.1:5700"
	}
	return &cfg, nil
}
