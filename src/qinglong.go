package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// QLClient 只走青龙面板官方 OpenAPI（/open/*），不调用 CLI、不拼 shell。
type QLClient struct {
	cfg   QingLongConfig
	http  *http.Client
	mu    sync.Mutex
	token string
	// id -> 任务名缓存：list_tasks 时写入，确认卡片用真实名称而不是只有 ID
	names map[string]string
}

func newQLClient(cfg QingLongConfig) *QLClient {
	return &QLClient{
		cfg:   cfg,
		http:  &http.Client{Timeout: 60 * time.Second},
		names: map[string]string{},
	}
}

func (c *QLClient) authToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Token != "" {
		return c.cfg.Token, nil
	}
	if c.token != "" {
		return c.token, nil
	}
	if c.cfg.ClientID == "" || c.cfg.ClientSecret == "" {
		return "", fmt.Errorf("青龙未配置 token，也未配置 clientId/clientSecret")
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/open/auth/token?client_id=" +
		url.QueryEscape(c.cfg.ClientID) + "&client_secret=" + url.QueryEscape(c.cfg.ClientSecret)
	// 青龙 2.x 的应用 token 接口是 GET（凭据走查询参数），POST 会返回 404
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("换取青龙 token 失败：HTTP %d %s", resp.StatusCode, truncate(string(data), 200))
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("token 响应解析失败")
	}
	token := ""
	if v, ok := parsed["token"].(string); ok {
		token = v
	}
	if token == "" {
		if d, ok := parsed["data"].(map[string]any); ok {
			if v, ok := d["token"].(string); ok {
				token = v
			}
		}
	}
	if token == "" {
		return "", fmt.Errorf("token 响应中没有 token 字段")
	}
	c.token = token
	return token, nil
}

func (c *QLClient) do(method, path string, query map[string]string, body any) (string, error) {
	token, err := c.authToken()
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + path
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			if strings.TrimSpace(v) != "" {
				q.Set(k, v)
			}
		}
		if enc := q.Encode(); enc != "" {
			endpoint += "?" + enc
		}
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("青龙返回 HTTP %d：%s", resp.StatusCode, truncate(string(data), 300))
	}
	return string(data), nil
}

func (c *QLClient) get(path string, query map[string]string, maxChars int) (string, error) {
	body, err := c.do(http.MethodGet, path, query, nil)
	if err != nil {
		return "", err
	}
	return sanitize(body, maxChars), nil
}

func (c *QLClient) executeTool(name string, args map[string]any) (string, error) {
	switch name {
	case "server_status":
		return c.get("/open/system", nil, 3000)
	case "list_tasks":
		raw, err := c.do(http.MethodGet, "/open/crons", map[string]string{
			"searchValue": str(args["search"]),
			"page":        def(str(args["page"]), "1"),
			"size":        def(str(args["limit"]), "20"),
		}, nil)
		if err != nil {
			return "", err
		}
		c.cacheTaskNames(raw)
		return sanitize(raw, 3000), nil
	case "today_runs":
		return c.get("/open/dashboard/top-count", nil, 3000)
	case "today_successes":
		return c.get("/open/dashboard/successes", nil, 3000)
	case "today_failures":
		return c.get("/open/dashboard/failures", nil, 3000)
	case "runtime_status":
		return c.get("/open/dashboard/runtime", nil, 3000)
	case "task_trend":
		return c.get("/open/dashboard/trend", map[string]string{
			"days": def(str(args["days"]), "7"),
		}, 3000)
	case "label_stats":
		return c.get("/open/dashboard/labels", nil, 3000)
	case "task_instances":
		return c.get("/open/crons/"+url.PathEscape(str(args["id"]))+"/instances", nil, 2000)
	case "list_envs":
		return c.get("/open/envs", map[string]string{
			"searchValue": str(args["search"]),
			"limit":       def(str(args["limit"]), "20"),
		}, 3000)
	case "list_scripts":
		return c.get("/open/scripts/files", map[string]string{
			"limit": def(str(args["limit"]), "20"),
		}, 3000)
	case "task_log":
		// 任务日志正文；不是 /open/logs 的日志文件清单。
		body, err := c.get("/open/crons/"+url.PathEscape(str(args["id"]))+"/log", nil, 0)
		if err != nil {
			return "", err
		}
		// 日志的结论/汇总通常在末尾，截断必须保留尾部。
		return truncateTail(body, 6000), nil
	case "list_logs":
		return c.listLogs(str(args["keyword"]), intArg(args["limit"], 40))
	case "read_log":
		return c.readLog(str(args["file"]), str(args["keyword"]), str(args["date"]))
	case "open_api":
		return c.openAPI(str(args["method"]), str(args["path"]), args["query"], args["body"])
	case "run_task":
		id := str(args["id"])
		if _, err := c.do(http.MethodPut, "/open/crons/run", nil, []any{toNumber(id)}); err != nil {
			return "", err
		}
		// run 只返回 accepted，补读日志让模型能总结真实结果。
		logText := ""
		for attempt := 0; attempt < 2; attempt++ {
			wait := 4 * time.Second
			if attempt > 0 {
				wait = 3 * time.Second
			}
			time.Sleep(wait)
			body, err := c.get("/open/crons/"+url.PathEscape(id)+"/log", nil, 4000)
			if err == nil && strings.TrimSpace(body) != "" {
				logText = body
				break
			}
		}
		if logText == "" {
			return "任务已提交运行（accepted），但暂时读不到日志；请稍后再查任务日志。", nil
		}
		return "任务已提交运行（accepted）。最新日志：\n" + logText, nil
	case "stop_task":
		if _, err := c.do(http.MethodPut, "/open/crons/stop", nil,
			[]any{toNumber(str(args["id"]))}); err != nil {
			return "", err
		}
		return "停止请求已提交（accepted）。", nil
	}
	return "", fmt.Errorf("未知工具：%s", name)
}

type logEntry struct {
	key  string
	size int64
}

// logFiles 把 /open/logs 的目录树压成扁平文件列表（按 key 倒序，最新在前）。
func (c *QLClient) logFiles() ([]logEntry, error) {
	raw, err := c.do(http.MethodGet, "/open/logs", nil, nil)
	if err != nil {
		return nil, err
	}
	var root struct {
		Data []struct {
			Children []struct {
				Key  string `json:"key"`
				Size int64  `json:"size"`
			} `json:"children"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return nil, fmt.Errorf("日志列表解析失败")
	}
	entries := make([]logEntry, 0, 64)
	for _, dir := range root.Data {
		for _, file := range dir.Children {
			if file.Key == "" {
				continue
			}
			entries = append(entries, logEntry{key: file.Key, size: file.Size})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key > entries[j].key })
	return entries, nil
}

// listLogs 列出日志文件，支持 keyword 过滤。
func (c *QLClient) listLogs(keyword string, limit int) (string, error) {
	all, err := c.logFiles()
	if err != nil {
		return "", err
	}
	entries := make([]logEntry, 0, len(all))
	for _, e := range all {
		if keyword != "" && !strings.Contains(e.key, keyword) {
			continue
		}
		entries = append(entries, e)
	}
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	total := len(entries)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "共 %d 个日志文件（按时间倒序，展示 %d 条）：\n", total, len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "%s  (%d 字节)\n", e.key, e.size)
	}
	return b.String(), nil
}

// findLog 按任务名/日期找最新匹配的日志文件。
func (c *QLClient) findLog(keyword, date string) string {
	entries, err := c.logFiles()
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if keyword != "" && !strings.Contains(e.key, keyword) {
			continue
		}
		if date != "" && !strings.Contains(e.key, date) {
			continue
		}
		return e.key
	}
	return ""
}

// readLog 读指定日志正文；file 为空时用 keyword/date 自动找最新匹配。
func (c *QLClient) readLog(file, keyword, date string) (string, error) {
	file = strings.TrimPrefix(strings.TrimSpace(file), "/")
	if file == "" {
		file = c.findLog(keyword, date)
		if file == "" {
			return "没有找到匹配的日志文件，请换一个 keyword 或 date（如 2026-11-13）。", nil
		}
	}
	dir, name := "", file
	if i := strings.LastIndex(file, "/"); i >= 0 {
		dir, name = file[:i], file[i+1:]
	}
	raw, err := c.do(http.MethodGet, "/open/logs/detail",
		map[string]string{"path": dir, "file": name}, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		Data string `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return "", fmt.Errorf("日志内容解析失败")
	}
	if strings.TrimSpace(out.Data) == "" {
		return "（日志为空）", nil
	}
	return "读取：" + file + "\n" + truncateTail(out.Data, 6000), nil
}

// openAPIRoots 是 open_api 允许访问的资源根，防止被用来打任意接口。
var openAPIRoots = map[string]bool{
	"crons": true, "subscriptions": true, "apps": true, "envs": true,
	"configs": true, "scripts": true, "dependencies": true, "system": true,
	"dashboard": true, "logs": true, "users": true, "auth": true,
}

// openAPI 是通用 OpenAPI 请求，path 必须落在白名单资源内。
func (c *QLClient) openAPI(method, path string, query, body any) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "GET", "POST", "PUT", "DELETE":
	default:
		return "", fmt.Errorf("不支持的 method：%s（仅 GET/POST/PUT/DELETE）", method)
	}
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "open/")
	if path == "" {
		return "", fmt.Errorf("path 不能为空")
	}
	root := path
	if i := strings.Index(path, "/"); i >= 0 {
		root = path[:i]
	}
	if !openAPIRoots[root] {
		return "", fmt.Errorf("path 不在白名单：%s", root)
	}
	q := map[string]string{}
	if m, ok := query.(map[string]any); ok {
		for k, v := range m {
			if s := str(v); s != "" {
				q[k] = s
			}
		}
	}
	raw, err := c.do(method, "/open/"+path, q, body)
	if err != nil {
		return "", err
	}
	return sanitize(raw, 6000), nil
}

func toolDefs() []ToolDef {
	emptySchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
	makeSchema := func(props map[string]any, required ...string) map[string]any {
		if required == nil {
			required = []string{}
		}
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	stringProp := map[string]any{"type": "string"}
	intProp := map[string]any{"type": "integer", "minimum": 1}

	makeTool := func(name, desc string, params map[string]any) ToolDef {
		return ToolDef{Type: "function", Function: FunctionDef{
			Name: name, Description: desc, Parameters: params,
		}}
	}

	return []ToolDef{
		makeTool("server_status", "读取青龙面板系统信息与当前状态。", emptySchema),
		makeTool("list_tasks", "按任务名称模糊搜索「任务定义」列表。只在定位某个脚本/任务或查看任务配置时用；不要用它回答「今天运行了哪些任务」。",
			makeSchema(map[string]any{"search": stringProp, "page": intProp, "limit": intProp})),
		makeTool("today_runs", "今日运行次数最多的任务 Top 列表。用户问「今天运行了哪些任务/今天跑了什么」时必须优先用它。", emptySchema),
		makeTool("today_successes", "今日成功执行的任务明细。", emptySchema),
		makeTool("today_failures", "今日失败任务明细（按失败次数降序）。", emptySchema),
		makeTool("runtime_status", "实时运行态：运行中/排队中的任务、24 小时未运行的任务。", emptySchema),
		makeTool("task_trend", "近 N 日任务执行趋势。", makeSchema(map[string]any{"days": intProp})),
		makeTool("label_stats", "按标签统计的任务执行情况。", emptySchema),
		makeTool("task_instances", "指定任务的运行实例列表。", makeSchema(map[string]any{"id": stringProp}, "id")),
		makeTool("list_envs", "列出环境变量元数据（值已脱敏）。", makeSchema(map[string]any{"search": stringProp, "limit": intProp})),
		makeTool("list_scripts", "列出脚本与目录元数据，不读取正文。", makeSchema(map[string]any{"limit": intProp})),
		makeTool("task_log", "读取指定任务的最新运行日志正文（不是日志文件清单，只保留尾部）。用户问「某任务跑了什么/成功失败原因」时用它。",
			makeSchema(map[string]any{"id": stringProp}, "id")),
		makeTool("list_logs", "列出青龙的日志文件（按时间倒序），用于查找历史某次运行的日志。keyword 可过滤任务名。",
			makeSchema(map[string]any{"keyword": stringProp, "limit": intProp})),
		makeTool("read_log", "读取日志正文（保留尾部）。file 可留空，改传 keyword（任务名/脚本名）与 date（如 2026-11-13）自动找最新匹配的日志；file 与 keyword 至少给一个。",
			makeSchema(map[string]any{"file": stringProp, "keyword": stringProp, "date": stringProp})),
		makeTool("open_api", "通用青龙 OpenAPI 请求，仅在现有工具覆盖不到时用（订阅、应用、依赖、配置、通知日志等）。path 如 crons/views、subscriptions、apps、envs、configs、scripts、dependencies、system、dashboard、logs、users，不要带 /open 前缀；写操作会先让用户确认。",
			makeSchema(map[string]any{
				"method": stringProp,
				"path":   stringProp,
				"query":  map[string]any{"type": "object"},
				"body":   map[string]any{"type": "object"},
			}, "method", "path")),
		makeTool("run_task", "运行一个定时任务。调用时必须把任务名称放进 name，便于用户确认；需要用户确认后才会执行。",
			makeSchema(map[string]any{"id": intProp, "name": stringProp}, "id")),
		makeTool("stop_task", "停止一个定时任务。调用时必须把任务名称放进 name，便于用户确认；需要用户确认后才会执行。",
			makeSchema(map[string]any{"id": intProp, "name": stringProp}, "id")),
	}
}

func requiresConfirmation(name string) bool {
	return name == "run_task" || name == "stop_task"
}

// cacheTaskNames 从 list_tasks 结果中缓存 id -> 任务名，供确认卡片使用。
func (c *QLClient) cacheTaskNames(raw string) {
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return
	}
	items := extractList(root)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.names == nil {
		c.names = map[string]string{}
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(m, "id", "_id")
		name := firstString(m, "name")
		if id != "" && name != "" {
			c.names[id] = name
		}
	}
}

// taskName 先查缓存，未命中再向青龙查单个任务。
func (c *QLClient) taskName(id string) string {
	if id == "" {
		return ""
	}
	c.mu.Lock()
	name := c.names[id]
	c.mu.Unlock()
	if name != "" {
		return name
	}
	raw, err := c.get("/open/crons/"+url.PathEscape(id), nil, 2000)
	if err != nil {
		return ""
	}
	var root map[string]any
	if json.Unmarshal([]byte(raw), &root) != nil {
		return ""
	}
	if d, ok := root["data"].(map[string]any); ok {
		if n := firstString(d, "name"); n != "" {
			return n
		}
	}
	return firstString(root, "name")
}

func summarizeAction(tool string, args map[string]any, taskName string) string {
	id := str(args["id"])
	label := ""
	if taskName != "" {
		label = "「" + taskName + "」"
	}
	switch tool {
	case "run_task":
		if label != "" {
			return "运行任务" + label + "（#" + id + "）"
		}
		return "运行任务 #" + id
	case "stop_task":
		if label != "" {
			return "停止任务" + label + "（#" + id + "）"
		}
		return "停止任务 #" + id
	}
	return tool + " #" + id
}

func parseCandidates(raw string) []Candidate {
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil
	}
	items := extractList(root)
	out := make([]Candidate, 0, 8)
	for _, item := range items {
		if len(out) >= 8 {
			break
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(m, "id", "_id")
		title := firstString(m, "name", "command")
		if title == "" {
			title = id
		}
		parts := make([]string, 0, 2)
		if v := firstString(m, "status"); v != "" {
			parts = append(parts, v)
		}
		if v := firstString(m, "schedule"); v != "" {
			parts = append(parts, v)
		}
		out = append(out, Candidate{ID: id, Title: title, Subtitle: strings.Join(parts, " · ")})
	}
	return out
}

func extractList(root any) []any {
	m, ok := root.(map[string]any)
	if !ok {
		if list, ok := root.([]any); ok {
			return list
		}
		return nil
	}
	if list, ok := m["data"].([]any); ok {
		return list
	}
	if d, ok := m["data"].(map[string]any); ok {
		if list, ok := d["data"].([]any); ok {
			return list
		}
	}
	return nil
}

var sensitivePattern = regexp.MustCompile(`(?i)"(value|token|client_secret|password|secret|cookie|pt_key|pt_pin|authorization)"\s*:\s*"[^"]*"`)

func sanitize(raw string, maxChars int) string {
	text := sensitivePattern.ReplaceAllString(raw, `"$1":"***"`)
	return truncate(text, maxChars)
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n…(内容已截断，仅展示前 %d 字符)", max)
}

// truncateTail 保留尾部：日志的结论/汇总通常在最后，不能砍尾。
func truncateTail(s string, maxRunes int) string {
	runes := []rune(s)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return s
	}
	return fmt.Sprintf("…(前面 %d 字符已省略)\n", len(runes)-maxRunes) +
		string(runes[len(runes)-maxRunes:])
}

func intArg(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n
		}
	}
	return fallback
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			case float64:
				return strconv.FormatFloat(t, 'f', -1, 64)
			}
		}
	}
	return ""
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

func def(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func toNumber(s string) any {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return s
}
