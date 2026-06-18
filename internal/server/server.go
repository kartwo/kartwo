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
	"os"
	"path/filepath"
	"time"

	adminui "github.com/kartwo/kartwo/web/admin"

	"github.com/kartwo/kartwo/internal/admin"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/storefront"
	"github.com/kartwo/kartwo/internal/store"
)

// New 构建带中间件的 HTTP Handler。
// 路由布局：店面占 "/"（SEO 主位）；Admin SPA 在 "/admin/"；API 在 "/admin/api/"；媒体在 "/media/"。
func New(cfg *config.Config, st *store.Store, version string, adminHTTP *admin.HTTP, storeHTTP *storefront.HTTP) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(st, version))
	adminHTTP.Register(mux)   // /admin/api/*（含商品/媒体 API）
	storeHTTP.Register(mux)   // /、/p/{slug}、/sitemap.xml、/robots.txt
	mux.Handle("GET /media/", mediaHandler(filepath.Join(cfg.DataDir, "media")))
	mux.Handle("/admin/", adminHandler()) // Admin SPA（/admin 自动跳 /admin/）

	return securityHeaders(cfg)(mux)
}

// mediaHandler 公开只读托管 ./data/media（供 Admin/店面展图）；禁目录列举。
func mediaHandler(root string) http.Handler {
	fileServer := http.FileServer(http.Dir(root))
	return http.StripPrefix("/media/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 禁止目录列举：以 / 结尾或映射到目录的请求返回 404。
		if len(r.URL.Path) == 0 || r.URL.Path[len(r.URL.Path)-1] == '/' {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(filepath.Join(root, filepath.Clean("/"+r.URL.Path))); err == nil && info.IsDir() {
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	}))
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

// adminHandler 在 /admin/ 前缀下托管内嵌的 Admin SPA（Vite base=/admin/）。
func adminHandler() http.Handler {
	sub, err := fs.Sub(adminui.FS, "dist")
	if err != nil {
		// 构建期保证 dist 存在；运行期 panic 即配置错误。
		panic(err)
	}
	return http.StripPrefix("/admin/", http.FileServer(http.FS(sub)))
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
			// script 严格 'self'（Vue 生产构建无需 eval/inline）；style 放开内联（Vue 内联 style 属性）；图片含本地 /media。
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'")
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
