package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	qqAPIBase  = "https://api.sgroup.qq.com"
	qqTokenURL = "https://bots.qq.com/app/getAppAccessToken"
	// GROUP_AND_C2C_EVENT：群聊 @ 与私聊消息
	qqIntentC2C = 1 << 25
)

// QQBot 用官方 WebSocket 网关接收私聊/群 @ 消息，不需要公网 IP。
// 消息交由同一个 Agent 处理，写操作在 QQ 里回复「确认」才执行。
type QQBot struct {
	cfg   QQBotConfig
	agent *Agent
	http  *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
	seq   int64

	pending sync.Map // sessionID -> actionID
	atRe    *regexp.Regexp
}

func newQQBot(cfg QQBotConfig, agent *Agent) *QQBot {
	if cfg.Intents == 0 {
		cfg.Intents = qqIntentC2C
	}
	return &QQBot{
		cfg:   cfg,
		agent: agent,
		http:  &http.Client{Timeout: 30 * time.Second},
		atRe:  regexp.MustCompile(`<@!?\d+>`),
	}
}

// Run 保持长连接，断了按退避重连。
func (q *QQBot) Run(ctx context.Context) {
	backoff := 5 * time.Second
	for {
		started := time.Now()
		if err := q.session(ctx); err != nil && ctx.Err() == nil {
			log.Printf("qqbot: 连接中断：%v", err)
		}
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > time.Minute {
			backoff = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (q *QQBot) session(ctx context.Context) error {
	token, err := q.accessToken()
	if err != nil {
		return err
	}
	gateway, err := q.gateway(token)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, gateway, nil)
	if err != nil {
		return fmt.Errorf("连接网关失败：%w", err)
	}
	defer conn.Close()

	var writeMu sync.Mutex
	write := func(payload any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(payload)
	}

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame struct {
			Op int             `json:"op"`
			S  int64           `json:"s"`
			T  string          `json:"t"`
			D  json.RawMessage `json:"d"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			continue
		}

		switch frame.Op {
		case 10: // hello
			var hello struct {
				Interval int `json:"heartbeat_interval"`
			}
			_ = json.Unmarshal(frame.D, &hello)
			if hello.Interval <= 0 {
				hello.Interval = 30000
			}
			if err := write(map[string]any{
				"op": 2,
				"d": map[string]any{
					"token":   "QQBot " + token,
					"intents": q.cfg.Intents,
					"shard":   []int{0, 1},
				},
			}); err != nil {
				return err
			}
			go q.heartbeat(ctx, time.Duration(hello.Interval)*time.Millisecond, write)

		case 0: // dispatch
			q.mu.Lock()
			q.seq = frame.S
			q.mu.Unlock()

			switch frame.T {
			case "READY":
				log.Printf("qqbot: 已连接（appId=%s）", q.cfg.AppID)
			case "C2C_MESSAGE_CREATE":
				var d struct {
					ID      string `json:"id"`
					Content string `json:"content"`
					Author  struct {
						UserOpenID string `json:"user_openid"`
					} `json:"author"`
				}
				if json.Unmarshal(frame.D, &d) == nil && d.Author.UserOpenID != "" {
					go q.handle(ctx, token, d.ID, d.Content, "user", d.Author.UserOpenID)
				}
			case "GROUP_AT_MESSAGE_CREATE":
				var d struct {
					ID          string `json:"id"`
					Content     string `json:"content"`
					GroupOpenID string `json:"group_openid"`
					Author      struct {
						MemberOpenID string `json:"member_openid"`
					} `json:"author"`
				}
				if json.Unmarshal(frame.D, &d) == nil && d.GroupOpenID != "" {
					go q.handle(ctx, token, d.ID, d.Content, "group", d.GroupOpenID)
				}
			}

		case 7:
			return fmt.Errorf("服务端要求重连")
		case 9:
			return fmt.Errorf("会话无效，需要重新 identify")
		}
	}
}

func (q *QQBot) heartbeat(ctx context.Context, interval time.Duration, write func(any) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.mu.Lock()
			seq := q.seq
			q.mu.Unlock()
			if err := write(map[string]any{"op": 1, "d": seq}); err != nil {
				return
			}
		}
	}
}

func (q *QQBot) accessToken() (string, error) {
	q.mu.Lock()
	if q.token != "" && time.Now().Before(q.exp) {
		token := q.token
		q.mu.Unlock()
		return token, nil
	}
	q.mu.Unlock()

	body, _ := json.Marshal(map[string]string{
		"appId":        q.cfg.AppID,
		"clientSecret": q.cfg.AppSecret,
	})
	resp, err := q.http.Post(qqTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取 access_token 失败：HTTP %d %s",
			resp.StatusCode, truncate(string(data), 200))
	}
	var out struct {
		AccessToken string      `json:"access_token"`
		ExpiresIn   json.Number `json:"expires_in"`
	}
	if json.Unmarshal(data, &out) != nil || out.AccessToken == "" {
		return "", fmt.Errorf("access_token 响应异常：%s", truncate(string(data), 200))
	}
	ttl := 7000
	if v, err := strconv.Atoi(out.ExpiresIn.String()); err == nil && v > 60 {
		ttl = v - 120
	}
	q.mu.Lock()
	q.token = out.AccessToken
	q.exp = time.Now().Add(time.Duration(ttl) * time.Second)
	q.mu.Unlock()
	return out.AccessToken, nil
}

func (q *QQBot) gateway(token string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, qqAPIBase+"/gateway", nil)
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := q.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取网关失败：HTTP %d %s", resp.StatusCode, truncate(string(data), 200))
	}
	var out struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(data, &out) != nil || out.URL == "" {
		return "", fmt.Errorf("网关响应异常：%s", truncate(string(data), 200))
	}
	return out.URL, nil
}

func (q *QQBot) handle(ctx context.Context, token, msgID, content, kind, target string) {
	text := strings.TrimSpace(q.atRe.ReplaceAllString(content, ""))
	if text == "" {
		return
	}
	sessionID := kind + ":" + target

	switch text {
	case "确认", "确定", "是":
		if value, ok := q.pending.Load(sessionID); ok {
			q.pending.Delete(sessionID)
			events := q.agent.HandleConfirm(sessionID, PendingAction{ID: value.(string)}, true)
			q.reply(ctx, token, msgID, kind, target, qqText(events))
			return
		}
	case "取消", "否":
		if value, ok := q.pending.Load(sessionID); ok {
			q.pending.Delete(sessionID)
			events := q.agent.HandleConfirm(sessionID, PendingAction{ID: value.(string)}, false)
			q.reply(ctx, token, msgID, kind, target, qqText(events))
			return
		}
	}

	events := q.agent.HandleMessage(sessionID, text, nil)
	for _, event := range events {
		if event.Type == eventNeedConfirmation && event.Pending != nil {
			q.pending.Store(sessionID, event.Pending.ID)
		}
	}
	q.reply(ctx, token, msgID, kind, target, qqText(events))
}

// qqText 把事件流压成一段适合 QQ 的纯文本。
func qqText(events []Event) string {
	parts := make([]string, 0, 3)
	for _, event := range events {
		switch event.Type {
		case eventMessage:
			if event.Text != "" {
				parts = append(parts, event.Text)
			}
		case eventNeedConfirmation:
			parts = append(parts, event.Text+"\n回复「确认」执行，回复「取消」放弃。")
		case eventCandidates:
			var b strings.Builder
			b.WriteString(event.Text)
			for i, c := range event.Candidates {
				if i >= 5 {
					break
				}
				fmt.Fprintf(&b, "\n%d. %s（#%s）", i+1, c.Title, c.ID)
			}
			parts = append(parts, b.String())
		case eventError:
			parts = append(parts, "出错了："+event.Text)
		}
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if out == "" {
		out = "（已处理，但模型没有返回文本）"
	}
	runes := []rune(out)
	if len(runes) > 1200 {
		out = string(runes[:1200]) + "…"
	}
	return out
}

func (q *QQBot) reply(ctx context.Context, token, msgID, kind, target, content string) {
	path := "/v2/users/" + target + "/messages"
	if kind == "group" {
		path = "/v2/groups/" + target + "/messages"
	}
	body, _ := json.Marshal(map[string]any{
		"content":  content,
		"msg_type": 0,
		"msg_id":   msgID,
	})
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, qqAPIBase+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "QQBot "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Union-Appid", q.cfg.AppID)
	resp, err := q.http.Do(req)
	if err != nil {
		log.Printf("qqbot: 回复失败：%v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("qqbot: 回复失败 HTTP %d %s", resp.StatusCode, truncate(string(data), 200))
	}
}
