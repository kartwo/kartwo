// 博客富文本安全渲染 / Blog Rich Text Sanitization
// 功能：只保留文章排版标签、安全链接和本地博客图片，过滤粘贴的脚本与属性
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 22:00:00
package blog

import (
	"html/template"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// SanitizeHTML 返回固定白名单内的正文，并指出是否有可见内容。
func SanitizeHTML(src string) (string, bool) {
	context := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(src), context)
	if err != nil {
		return "", false
	}
	var out strings.Builder
	visible := false
	for _, node := range nodes {
		appendSafeHTML(&out, node, &visible)
	}
	return out.String(), visible
}

func attr(node *html.Node, key string) string {
	for _, a := range node.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func appendSafeHTML(out *strings.Builder, node *html.Node, visible *bool) {
	switch node.Type {
	case html.TextNode:
		out.WriteString(template.HTMLEscapeString(node.Data))
		if strings.TrimSpace(node.Data) != "" {
			*visible = true
		}
		return
	case html.ElementNode:
	default:
		return
	}

	tag := strings.ToLower(node.Data)
	switch tag {
	case "script", "style", "iframe", "object", "embed", "svg", "math", "template", "form", "textarea", "button", "input":
		return
	case "img":
		src := attr(node, "src")
		if strings.HasPrefix(src, "/media/blog/") && imagePathPattern.MatchString(strings.TrimPrefix(src, "/media/")) {
			out.WriteString(`<img src="` + template.HTMLEscapeString(src) + `" alt="` + template.HTMLEscapeString(attr(node, "alt")) + `" loading="lazy">`)
			*visible = true
		}
		return
	case "br":
		out.WriteString("<br>")
		return
	}

	allowed := map[string]string{
		"p": "p", "div": "p", "h1": "h2", "h2": "h2", "h3": "h3",
		"ul": "ul", "ol": "ol", "li": "li", "blockquote": "blockquote",
		"strong": "strong", "b": "strong", "em": "em", "i": "em",
	}
	if tag == "a" {
		href := attr(node, "href")
		if safeLink(href) {
			out.WriteString(`<a href="` + template.HTMLEscapeString(href) + `" rel="noopener noreferrer">`)
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				appendSafeHTML(out, child, visible)
			}
			out.WriteString("</a>")
			return
		}
	}
	cleanTag := allowed[tag]
	if cleanTag != "" {
		out.WriteString("<" + cleanTag + ">")
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendSafeHTML(out, child, visible)
	}
	if cleanTag != "" {
		out.WriteString("</" + cleanTag + ">")
	}
}

// RenderHTML 读取时再过滤一次，避免数据库中已有不可信 HTML 被直接送到页面。
func RenderHTML(src string) template.HTML {
	safe, _ := SanitizeHTML(src)
	return template.HTML(safe) //nolint:gosec // only fixed allowlisted tags and escaped text/attributes remain
}
