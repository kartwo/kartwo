// 博客富文本安全渲染 / Blog Rich Text Sanitization
// 功能：只保留文章排版标签、安全链接和本地博客图片，过滤粘贴的脚本与属性
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 22:00:00
package blog

import (
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var youtubeVideoID = regexp.MustCompile(`^/embed/[A-Za-z0-9_-]{11}$`)
var vimeoVideoID = regexp.MustCompile(`^/video/[0-9]{1,20}$`)

func safeVideoEmbed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "www.youtube-nocookie.com":
		return youtubeVideoID.MatchString(u.Path)
	case "player.vimeo.com":
		return vimeoVideoID.MatchString(u.Path)
	default:
		return false
	}
}

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

func safeColor(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if len(value) == 7 && value[0] == '#' {
		for _, char := range value[1:] {
			if !strings.ContainsRune("0123456789abcdef", char) {
				return ""
			}
		}
		return value
	}
	if !strings.HasPrefix(value, "rgb(") || !strings.HasSuffix(value, ")") {
		return ""
	}
	parts := strings.Split(value[4:len(value)-1], ",")
	if len(parts) != 3 {
		return ""
	}
	var rgb [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return ""
		}
		rgb[i] = n
	}
	const hex = "0123456789abcdef"
	return string([]byte{'#', hex[rgb[0]/16], hex[rgb[0]%16], hex[rgb[1]/16], hex[rgb[1]%16], hex[rgb[2]/16], hex[rgb[2]%16]})
}

func safeStyle(node *html.Node) string {
	styles := map[string]string{}
	for _, item := range strings.Split(attr(node, "style"), ";") {
		key, value, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(strings.ToLower(key)), strings.TrimSpace(strings.ToLower(value))
		switch key {
		case "color", "background-color":
			if color := safeColor(value); color != "" {
				styles[key] = color
			}
		case "font-family":
			if value == "serif" || value == "sans-serif" || value == "monospace" {
				styles[key] = value
			}
		case "font-size":
			size, err := strconv.Atoi(strings.TrimSuffix(value, "px"))
			if err == nil && strings.HasSuffix(value, "px") && size >= 12 && size <= 32 {
				styles[key] = strconv.Itoa(size) + "px"
			}
		case "line-height":
			if value == "1.2" || value == "1.5" || value == "1.8" || value == "2" {
				styles[key] = value
			}
		case "text-align":
			if value == "left" || value == "center" || value == "right" || value == "justify" {
				styles[key] = value
			}
		}
	}
	if node.Data == "font" {
		if color := safeColor(attr(node, "color")); color != "" {
			styles["color"] = color
		}
		if family := strings.ToLower(attr(node, "face")); family == "serif" || family == "sans-serif" || family == "monospace" {
			styles["font-family"] = family
		}
		fontSizes := map[string]string{"2": "13px", "3": "16px", "4": "18px", "5": "24px", "6": "32px"}
		if size := fontSizes[attr(node, "size")]; size != "" {
			styles["font-size"] = size
		}
	}
	var parts []string
	for _, key := range []string{"color", "background-color", "font-family", "font-size", "line-height", "text-align"} {
		if value := styles[key]; value != "" {
			parts = append(parts, key+":"+value)
		}
	}
	return strings.Join(parts, ";")
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
	case "script", "style", "object", "embed", "svg", "math", "template", "form", "textarea", "button", "input":
		return
	case "img":
		src := attr(node, "src")
		if strings.HasPrefix(src, "/media/blog/") && imagePathPattern.MatchString(strings.TrimPrefix(src, "/media/")) {
			out.WriteString(`<img src="` + template.HTMLEscapeString(src) + `" alt="` + template.HTMLEscapeString(attr(node, "alt")) + `" loading="lazy">`)
			*visible = true
		}
		return
	case "iframe":
		src := attr(node, "src")
		if safeVideoEmbed(src) {
			out.WriteString(`<iframe src="` + template.HTMLEscapeString(src) + `" title="Video" loading="lazy" referrerpolicy="no-referrer" allowfullscreen></iframe>`)
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
		"strong": "strong", "b": "strong", "em": "em", "i": "em", "u": "u", "s": "s", "strike": "s", "del": "s",
		"span": "span", "font": "span", "table": "table", "thead": "thead", "tbody": "tbody", "tr": "tr", "th": "th", "td": "td",
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
		out.WriteString("<" + cleanTag)
		if style := safeStyle(node); style != "" {
			out.WriteString(` style="` + style + `"`)
		}
		out.WriteString(">")
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
