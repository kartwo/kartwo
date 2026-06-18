// Admin 鉴权中间件 / Admin Auth Middleware
// 功能：会话鉴权（防 IDOR 地基）+ 写操作 CSRF 双提交校验
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"context"
	"errors"
	"net/http"
)

type ctxKey int

const authCtxKey ctxKey = 0

// requireAuth 校验会话 cookie；非安全方法额外校验 CSRF（双提交）。失败即拦截。
func (h *HTTP) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		ac, err := h.svc.Authenticate(r.Context(), c.Value)
		if errors.Is(err, ErrUnauthorized) {
			writeErr(w, http.StatusUnauthorized, "会话无效或已过期")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "内部错误")
			return
		}

		// 写操作强制 CSRF：请求头 X-CSRF-Token 必须等于会话 csrf_token。
		if isUnsafe(r.Method) {
			if r.Header.Get(csrfHeader) == "" || r.Header.Get(csrfHeader) != ac.CSRFToken {
				writeErr(w, http.StatusForbidden, "CSRF 校验失败")
				return
			}
		}

		ctx := context.WithValue(r.Context(), authCtxKey, ac)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isUnsafe(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// authFrom 取出已鉴权身份（仅在 requireAuth 之后可用）。
func authFrom(ctx context.Context) *AuthContext {
	ac, _ := ctx.Value(authCtxKey).(*AuthContext)
	return ac
}

// RequireAuth 导出给其它包（如 M1.3 商品 CRUD）复用的鉴权中间件。
func (h *HTTP) RequireAuth(next http.Handler) http.Handler {
	return h.requireAuth(next)
}

// Auth 暴露请求上下文中的鉴权身份，供其它包读取。
func Auth(ctx context.Context) *AuthContext {
	return authFrom(ctx)
}
