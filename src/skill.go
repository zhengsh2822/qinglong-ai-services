package main

import (
	"os"
	"strings"
)

// 官方 Skill 的路由手册（@whyour/qinglong-cli 自带 skills/qinglong-cli）。
//
// 设计取舍：
//   - 能力放服务端（Core 以 root 运行，读得到 proot 容器内的 openapi.md），
//     客户端不用带知识、不用改包；
//   - 注入的是「精简路由表」而不是全文，token 可控；
//   - 文件读不到时回退到内置表，保证任何环境下 AI 都能按表查而不是靠猜。
const builtinCatalog = `任务：GET crons｜POST crons｜PUT crons｜DELETE crons｜POST crons/run｜POST crons/stop｜GET crons/views｜POST crons/views｜PUT crons/views｜DELETE crons/views｜GET crons/:id/log｜GET crons/:id/log/latest
订阅：GET subscriptions｜POST subscriptions｜PUT subscriptions｜DELETE subscriptions｜POST subscriptions/run｜GET subscriptions/:id/log
应用：GET apps｜POST apps｜PUT apps｜DELETE apps｜GET apps/:id
环境变量：GET envs｜POST envs｜PUT envs｜DELETE envs
配置：GET configs｜POST configs｜PUT configs
脚本：GET scripts｜POST scripts｜PUT scripts｜DELETE scripts｜POST scripts/run
依赖：GET dependencies｜POST dependencies｜PUT dependencies｜DELETE dependencies
系统：GET system/notify-log｜GET system/log｜GET system/env｜GET system/user｜GET dashboard｜GET dashboard/today｜GET logs｜GET logs/:file`

// LoadSkillCatalog 加载官方 Skill 路由手册。
// 优先解析 openapi.md 的表格；读不到/解析为空时用内置表。
func LoadSkillCatalog(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return builtinCatalog
	}
	lines := parseOpenAPIMD(string(data))
	if len(lines) == 0 {
		return builtinCatalog
	}
	return strings.Join(lines, "\n")
}

// parseOpenAPIMD 宽松解析 openapi.md：
// 只挑出含 HTTP method 的表格行，拼成「分组：METHOD path｜…」的一行。
// 不依赖列顺序，列名/描述都不要，保持紧凑省 token。
func parseOpenAPIMD(content string) []string {
	methods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true,
	}
	group := "其他"
	var out []string
	var buf []string

	flush := func() {
		if len(buf) > 0 {
			out = append(out, group+"："+strings.Join(buf, "｜"))
			buf = nil
		}
	}

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		// 标题行当分组（如 "### 任务 / crons"）
		if strings.HasPrefix(line, "#") {
			flush()
			group = strings.TrimLeft(line, "# ")
			if i := strings.IndexAny(group, "（("); i > 0 {
				group = strings.TrimSpace(group[:i])
			}
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitTableRow(line)
		var method, path string
		for _, c := range cells {
			u := strings.ToUpper(strings.TrimSpace(c))
			if methods[u] {
				method = u
				continue
			}
			// path 特征：含 / 且不含空格，形如 crons/:id、system/notify-log
			if method == "" && strings.Contains(c, "/") && !strings.Contains(c, " ") &&
				len(c) < 60 && !strings.Contains(c, "http") {
				path = strings.TrimSpace(c)
			}
		}
		if method == "" || path == "" {
			continue
		}
		path = strings.TrimPrefix(path, "/open/")
		path = strings.TrimPrefix(path, "/")
		buf = append(buf, method+" "+path)
	}
	flush()
	return out
}

func splitTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.Trim(p, "-: ") == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
