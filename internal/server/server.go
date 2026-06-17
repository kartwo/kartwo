// HTTP 服务装配 / HTTP Server
// 功能：路由、安全响应头中间件、健康检查、Admin SPA 占位托管（M0 骨架）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	adminui "github.com/kartwo/kartwo/web/admin"

	"github.com/kartwo/kartwo/internal/admin"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/store"
)

// New 构建带中间件的 HTTP Handler。
func New(cfg *config.Config, st *store.Store, version string, adminHTTP *admin.HTTP) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(st, version))
	adminHTTP.Register(mux) // /admin/api/*
	mux.Handle("/", adminHandler())

	return securityHeaders(cfg)(mux)
}

// healthHandler 返回服务存活与数据库可达状态。
func healthHandler(st *store.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"status":  "ok",
			"version": version,
			"time":    time.Now().UTC().Format(time.RFC3339),
		}
		if err := st.DB.PingContext(r.Context()); err != nil {
			resp["status"] = "degraded"
			resp["db"] = "unreachable"
			writeJSON(w, http.StatusServiceUnavailable, resp)
			return
		}
		resp["db"] = "ok"
		writeJSON(w, http.StatusOK, resp)
	}
}

// adminHandler 托管内嵌的 Admin SPA 占位静态资源。
func adminHandler() http.Handler {
	sub, err := fs.Sub(adminui.FS, "dist")
	if err != nil {
		// 构建期保证 dist 存在；运行期 panic 即配置错误。
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

// securityHeaders 注入强制安全响应头（HSTS/CSP/X-Frame-Options/X-Content-Type-Options 等）。
// HSTS 仅在 prod 注入，避免 dev 本地 HTTP 调试被浏览器强升 HTTPS。
func securityHeaders(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'self'")
			if cfg.Env == "prod" {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
