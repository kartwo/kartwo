// 私密响应保护 / Private Response Protection
// 功能：禁止个人与交易响应缓存、搜索索引和 Referer 外传；可包裹成功及错误响应
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:07:13
package httpx

import "net/http"

// PrivateHeaders applies before authentication, redirects and errors as well as
// successful responses. This is defense in depth, not an authorization check.
func PrivateHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func PrivateHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		PrivateHeaders(w)
		next(w, r)
	}
}
