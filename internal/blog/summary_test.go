// 博客列表摘要测试 / Blog List Summary Tests
// 功能：验证手写摘要优先、富文本和 Markdown 正文回退以及长度限制
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-14 10:50:00
package blog

import (
	"strings"
	"testing"
)

func TestListSummary(t *testing.T) {
	tests := []struct {
		name string
		post Post
		want string
	}{
		{"handwritten", Post{Excerpt: "  Merchant summary  ", BodyHTML: "<p>Body</p>"}, "Merchant summary"},
		{"rich text", Post{BodyHTML: "<p>First <strong>paragraph</strong> &amp; more.</p><p>Second.</p>"}, "First paragraph & more. Second."},
		{"unsafe HTML", Post{BodyHTML: "<p>Safe text</p><script>alert('unsafe')</script>"}, "Safe text"},
		{"markdown", Post{BodyMarkdown: "## Heading\n\nFirst paragraph."}, "Heading First paragraph."},
		{"empty", Post{}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.post.ListSummary(); got != test.want {
				t.Fatalf("ListSummary() = %q, want %q", got, test.want)
			}
		})
	}
	long := Post{BodyHTML: "<p>" + strings.Repeat("好", 200) + "</p>"}
	if got := long.ListSummary(); len([]rune(got)) != listSummaryRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long summary = %q", got)
	}
}
