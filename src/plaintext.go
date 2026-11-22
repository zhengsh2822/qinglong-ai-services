package main

import (
	"regexp"
	"strings"
)

var (
	mdBold      = regexp.MustCompile(`\*\*|__`)
	mdHeading   = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	mdTableRule = regexp.MustCompile(`^\|?[\s:\-|]+\|?$`)
)

// plainText 把模型偶尔输出的 Markdown 清成纯文本：
// 去掉加粗与标题前缀，表格行转成「· 列 · 列」，分隔行直接删除。
// 与 App 端 _plainText 保持同一套规则，保证 App / 远程 / 未来 QQ 显示一致。
func plainText(s string) string {
	if s == "" {
		return s
	}
	s = mdBold.ReplaceAllString(s, "")
	s = mdHeading.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 表格分隔行 |---|---|
		if trimmed != "" && strings.Contains(trimmed, "-") && mdTableRule.MatchString(trimmed) {
			continue
		}
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			cells := make([]string, 0, 4)
			for _, cell := range strings.Split(trimmed, "|") {
				if cell = strings.TrimSpace(cell); cell != "" {
					cells = append(cells, cell)
				}
			}
			if len(cells) > 0 {
				out = append(out, "· "+strings.Join(cells, " · "))
				continue
			}
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
