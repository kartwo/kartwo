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

func (p Post) renderedListBody() string {
	if p.BodyHTML != "" {
		return string(RenderHTML(p.BodyHTML))
	}
	return string(RenderMarkdown(p.BodyMarkdown))
}

// ListSummary prefers the merchant's excerpt and otherwise previews the post body.
func (p Post) ListSummary() string {
	if excerpt := strings.TrimSpace(p.Excerpt); excerpt != "" {
		return excerpt
	}
	tokenizer := html.NewTokenizer(strings.NewReader(p.renderedListBody()))
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

// ListImageURL uses the chosen cover, or the first safe local body image on list cards.
func (p Post) ListImageURL() string {
	if cover := p.CoverURL(); cover != "" {
		return cover
	}
	tokenizer := html.NewTokenizer(strings.NewReader(p.renderedListBody()))
	for tokenizer.Next() != html.ErrorToken {
		token := tokenizer.Token()
		if (token.Type != html.StartTagToken && token.Type != html.SelfClosingTagToken) || token.Data != "img" {
			continue
		}
		for _, attr := range token.Attr {
			if attr.Key == "src" && strings.HasPrefix(attr.Val, "/media/blog/") && imagePathPattern.MatchString(strings.TrimPrefix(attr.Val, "/media/")) {
				return attr.Val
			}
		}
	}
	return ""
}
