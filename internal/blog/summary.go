// 博客列表摘要 / Blog List Summary
// 功能：摘要留空时从已安全渲染的正文提取简短纯文字预览
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-14 10:50:00
package blog

import (
	"strings"

	"golang.org/x/net/html"
)

const listSummaryRunes = 180

// ListSummary prefers the merchant's excerpt and otherwise previews the post body.
func (p Post) ListSummary() string {
	if excerpt := strings.TrimSpace(p.Excerpt); excerpt != "" {
		return excerpt
	}
	var rendered string
	if p.BodyHTML != "" {
		rendered = string(RenderHTML(p.BodyHTML))
	} else {
		rendered = string(RenderMarkdown(p.BodyMarkdown))
	}
	tokenizer := html.NewTokenizer(strings.NewReader(rendered))
	var words []string
	for tokenizer.Next() != html.ErrorToken {
		token := tokenizer.Token()
		if token.Type == html.TextToken {
			words = append(words, token.Data)
		}
	}
	summary := strings.Join(strings.Fields(strings.Join(words, " ")), " ")
	runes := []rune(summary)
	if len(runes) > listSummaryRunes {
		return string(runes[:listSummaryRunes]) + "…"
	}
	return summary
}
