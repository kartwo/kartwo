// Admin HTTP 接口 / Admin HTTP Handlers
// 功能：向导初始化、登录、登出、me；会话/CSRF cookie；登录限流
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookie = "kartwo_session"
	csrfCookie    = "kartwo_csrf"
	csrfHeader    = "X-CSRF-Token"
	minPasswordLen = 8
)

// HTTP 承载 Admin API 处理器。
type HTTP struct {
	svc     *Service
	secure  bool          // prod 下 cookie 加 Secure
	limiter *loginLimiter
}

// NewHTTP 构建 Admin HTTP 层。secure=true 时 cookie 标记 Secure（prod）。
func NewHTTP(svc *Service, secure bool) *HTTP {
	return &HTTP{svc: svc, secure: secure, limiter: newLoginLimiter(5, time.Minute)}
}

// Register 在给定 mux 上注册 /admin/api/* 路由。
func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/api/status", h.status)
	mux.HandleFunc("POST /admin/api/setup", h.setup)
	mux.HandleFunc("POST /admin/api/login", h.login)
	mux.Handle("POST /admin/api/logout", h.requireAuth(http.HandlerFunc(h.logout)))
	mux.Handle("GET /admin/api/me", h.requireAuth(http.HandlerFunc(h.me)))
}

func (h *HTTP) status(w http.ResponseWriter, r *http.Request) {
	init, err := h.svc.IsInitialized(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"initialized": init})
}

func (h *HTTP) setup(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Password) < minPasswordLen {
		writeErr(w, http.StatusBadRequest, "口令至少 8 位")
		return
	}
	err := h.svc.Initialize(r.Context(), strings.TrimSpace(req.Username), req.Password)
	if errors.Is(err, ErrAlreadyInitialized) {
		writeErr(w, http.StatusConflict, "已初始化，不能重复设置")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (h *HTTP) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if !readJSON(w, r, &req) {
		return
	}
	key := clientIP(r) + "|" + strings.TrimSpace(req.Username)
	if !h.limiter.allow(key) {
		writeErr(w, http.StatusTooManyRequests, "登录尝试过多，请稍后再试")
		return
	}

	sess, err := h.svc.Login(r.Context(), strings.TrimSpace(req.Username), req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		writeErr(w, http.StatusUnauthorized, "用户名或口令错误")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	h.limiter.reset(key)

	h.setCookie(w, sessionCookie, sess.Token, sess.ExpiresAt, true)
	h.setCookie(w, csrfCookie, sess.CSRFToken, sess.ExpiresAt, false) // 非 HttpOnly，供 SPA 读取回传
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	if err := h.svc.Logout(r.Context(), ac.SessionToken); err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	h.clearCookie(w, sessionCookie, true)
	h.clearCookie(w, csrfCookie, false)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) me(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"username": ac.Username, "public_id": ac.AdminPublicID})
}

func (h *HTTP) setCookie(w http.ResponseWriter, name, value string, expires time.Time, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", Expires: expires,
		HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (h *HTTP) clearCookie(w http.ResponseWriter, name string, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体非法")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
