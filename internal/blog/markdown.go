// 博客正文渲染 / Blog Markdown Rendering
// 功能：仅输出经过转义的标题、段落、列表、安全链接和本地博客图片
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package blog

import (
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

var inlinePattern = regexp.MustCompile(`!\[([^\]\n]{0,250})\]\(([^)\s]+)\)|\[([^\]\n]{1,250})\]\(([^)\s]+)\)`)

func safeLink(raw string) bool {
	if strings.ContainsAny(raw, "<>\"'\\") {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func safeInline(src string) string {
	var b strings.Builder
	last := 0
	for _, loc := range inlinePattern.FindAllStringSubmatchIndex(src, -1) {
		b.WriteString(template.HTMLEscapeString(src[last:loc[0]]))
		whole := src[loc[0]:loc[1]]
		if strings.HasPrefix(whole, "![") {
			alt := src[loc[2]:loc[3]]
			u := src[loc[4]:loc[5]]
			if imagePathPattern.MatchString(strings.TrimPrefix(u, "/media/")) && strings.HasPrefix(u, "/media/blog/") {
				b.WriteString(`<img src="` + template.HTMLEscapeString(u) + `" alt="` + template.HTMLEscapeString(alt) + `" loading="lazy" />`)
			} else {
				b.WriteString(template.HTMLEscapeString(whole))
			}
		} else {
			label := src[loc[6]:loc[7]]
			u := src[loc[8]:loc[9]]
			if safeLink(u) {
				b.WriteString(`<a href="` + template.HTMLEscapeString(u) + `" rel="noopener noreferrer">` + template.HTMLEscapeString(label) + `</a>`)
			} else {
				b.WriteString(template.HTMLEscapeString(whole))
			}
		}
		last = loc[1]
	}
	b.WriteString(template.HTMLEscapeString(src[last:]))
	return b.String()
}

// RenderMarkdown 返回只含固定安全标签的 HTML；绝不执行原始 HTML。
func RenderMarkdown(src string) template.HTML {
	var b strings.Builder
	inList := false
	closeList := func() {
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			closeList()
			continue
		}
		switch {
		case strings.HasPrefix(line, "### "):
			closeList()
			b.WriteString("<h3>" + safeInline(line[4:]) + "</h3>")
		case strings.HasPrefix(line, "## "):
			closeList()
			b.WriteString("<h2>" + safeInline(line[3:]) + "</h2>")
		case strings.HasPrefix(line, "# "):
			closeList()
			b.WriteString("<h2>" + safeInline(line[2:]) + "</h2>")
		case strings.HasPrefix(line, "- "):
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + safeInline(line[2:]) + "</li>")
		default:
			closeList()
			b.WriteString("<p>" + safeInline(line) + "</p>")
		}
	}
	closeList()
	return template.HTML(b.String()) //nolint:gosec // all untrusted text is escaped before fixed tags are added
}
