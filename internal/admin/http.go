// Admin HTTP 接口 / Admin HTTP Handlers
// 功能：向导初始化、登录、登出、me；会话/CSRF cookie；登录限流
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kartwo/kartwo/internal/audit"
	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/httpx"
	"github.com/kartwo/kartwo/internal/importer"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/policy"
	"github.com/kartwo/kartwo/internal/settings"
)

// refundService 约束后台退款所需的最小能力，便于隔离 HTTP 审计边界与支付网关实现。
type refundService interface {
	Refund(ctx context.Context, orderPublicID string) error
}

// stripeConnectionTester 是收款页在线验证 Secret key 的可选能力。
type stripeConnectionTester interface {
	TestStripeConnection(ctx context.Context) error
}

const (
	sessionCookie  = "kartwo_session"
	csrfCookie     = "kartwo_csrf"
	csrfHeader     = "X-CSRF-Token"
	minPasswordLen = 8
)

// HTTP 承载 Admin API 处理器。
type HTTP struct {
	svc         *Service
	cat         *catalog.Service
	importer    *importer.Service
	media       *media.Service
	settings    *settings.Service
	policy      *policy.Service
	orders      *order.Service // 后台订单页（M3.3a 起）
	pay         refundService  // 退款编排（M3.3a 起），可为 nil
	stripeTest  stripeConnectionTester
	mailCache   *mail.Cache      // SMTP 凭证缓存（M4.3 设置页/测试发信/向导），可为 nil
	exporter    *backup.Exporter // 全量数据导出（M5.6），可为 nil
	audit       *audit.Service   // 关键后台动作的只追加审计记录（M6.1）
	backupCfg   BackupConfig     // 本地自动备份的有效配置与 env 覆盖状态（M5.12）
	envDomain   string           // KARTWO_DOMAIN（env 覆盖 DB 的域名来源，M4.2.1 域名步骤展示/只读判定）
	envShopName string           // KARTWO_SHOP_NAME（非空时覆盖 DB 店铺名称）
	secure      bool             // 本实例能否签发 HTTPS（prod=true，dev 恒 false），供 domain 页 https_capable
	limiter     *loginLimiter
	trusted     []*net.IPNet
	demo        DemoConfig
}

// DemoConfig 控制公开演示权限与资源上限；Enabled=false 时所有既有行为保持不变。
type DemoConfig struct {
	Enabled         bool
	SessionTTL      time.Duration
	MaxProducts     int
	MaxImageBytes   int64
	CleanupInterval time.Duration
}

// NewHTTP 构建 Admin HTTP 层。secure=true 表示本实例可启用 HTTPS（prod）；
// 注意 cookie 的 Secure 标记按**每次请求**是否走 TLS 决定（决策 D8-A），与此参数无关。
// envDomain=KARTWO_DOMAIN，非空时域名由 env 提供、后台只读（决策 C：env 覆盖 DB、不双写）。
func NewHTTP(svc *Service, cat *catalog.Service, importSvc *importer.Service, md *media.Service, settingsSvc *settings.Service, orderSvc *order.Service, paySvc refundService, mailCache *mail.Cache, exporter *backup.Exporter, backupCfg BackupConfig, envDomain string, secure bool, trusted []*net.IPNet, envShopName ...string) *HTTP {
	h := &HTTP{svc: svc, cat: cat, importer: importSvc, media: md, settings: settingsSvc, policy: policy.New(settingsSvc, cat), orders: orderSvc, pay: paySvc, mailCache: mailCache, exporter: exporter, audit: audit.New(svc.db), backupCfg: backupCfg, envDomain: envDomain, secure: secure, trusted: trusted, limiter: newLoginLimiter(5, time.Minute)}
	if tester, ok := paySvc.(stripeConnectionTester); ok {
		h.stripeTest = tester
	}
	if len(envShopName) > 0 {
		h.envShopName = envShopName[0]
	}
	return h
}

// ConfigureDemo 注入显式启用的公开演示策略。
func (h *HTTP) ConfigureDemo(cfg DemoConfig) {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 45 * time.Minute
	}
	if cfg.MaxProducts < 1 || cfg.MaxProducts > 3 {
		cfg.MaxProducts = 3
	}
	if cfg.MaxImageBytes <= 0 || cfg.MaxImageBytes > 2<<20 {
		cfg.MaxImageBytes = 2 << 20
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}
	h.demo = cfg
}

// Register 在给定 mux 上注册 /admin/api/* 路由。
func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/api/status", h.status)
	mux.HandleFunc("POST /admin/api/setup", h.setup)
	mux.HandleFunc("POST /admin/api/login", h.login)
	mux.HandleFunc("POST /admin/api/demo-session", h.demoSession)
	mux.Handle("POST /admin/api/logout", h.requireAuth(http.HandlerFunc(h.logout)))
	mux.Handle("GET /admin/api/me", h.requireAuth(http.HandlerFunc(h.me)))
	mux.Handle("POST /admin/api/demo/reset", h.requireAuth(http.HandlerFunc(h.resetDemo)))

	// 商品/分类/变体 CRUD（均需鉴权；写操作经中间件 CSRF 校验）。
	protect := func(fn http.HandlerFunc) http.Handler { return h.requireAuth(fn) }
	owner := func(fn http.HandlerFunc) http.Handler { return h.requireOwner(fn) }
	mux.Handle("GET /admin/api/products", protect(h.listProducts))
	mux.Handle("POST /admin/api/products", protect(h.createProduct))
	mux.Handle("GET /admin/api/products/{id}", protect(h.getProduct))
	mux.Handle("PATCH /admin/api/products/{id}", protect(h.updateProduct))
	mux.Handle("PATCH /admin/api/products/{id}/featured", owner(h.setProductFeatured))
	mux.Handle("DELETE /admin/api/products/{id}", protect(h.deleteProduct))
	mux.Handle("PATCH /admin/api/variants/{id}/inventory", protect(h.setVariantInventory))
	mux.Handle("PATCH /admin/api/variants/{id}/price", protect(h.setVariantPrice))
	mux.Handle("GET /admin/api/categories", protect(h.listCategories))
	mux.Handle("POST /admin/api/categories", owner(h.createCategory))
	mux.Handle("PATCH /admin/api/categories/{id}", owner(h.updateCategory))
	mux.Handle("DELETE /admin/api/categories/{id}", owner(h.deleteCategory))
	mux.Handle("GET /admin/api/content-pages", protect(h.listContentPages))
	mux.Handle("POST /admin/api/content-pages", owner(h.createContentPage))
	mux.Handle("GET /admin/api/content-pages/{id}", protect(h.getContentPage))
	mux.Handle("PATCH /admin/api/content-pages/{id}", owner(h.updateContentPage))
	mux.Handle("DELETE /admin/api/content-pages/{id}", owner(h.deleteContentPage))
	mux.Handle("POST /admin/api/content-pages/generate-footer", owner(h.generateFooterPages))
	mux.Handle("POST /admin/api/imports/csv/preview", owner(h.previewCSVImport))
	mux.Handle("POST /admin/api/imports/{id}/execute", owner(h.executeImport))
	mux.Handle("GET /admin/api/imports/{id}", owner(h.getImport))

	// 媒体上传/列表/删除。
	mux.Handle("POST /admin/api/products/{id}/media", protect(h.uploadMedia))
	mux.Handle("GET /admin/api/products/{id}/media", protect(h.listMedia))
	mux.Handle("DELETE /admin/api/media/{id}", protect(h.deleteMedia))
	mux.Handle("PATCH /admin/api/media/{id}", protect(h.updateMediaAlt))

	// 向导：主攻市场。
	mux.Handle("GET /admin/api/markets", protect(h.listMarkets))
	mux.Handle("GET /admin/api/settings/market", protect(h.getMarket))
	mux.Handle("PUT /admin/api/settings/market", owner(h.setMarket))

	// 收款设置（Stripe 密钥；sk/whsec 加密存）。
	mux.Handle("GET /admin/api/settings/payment", owner(h.getPayment))
	mux.Handle("PUT /admin/api/settings/payment", owner(h.setPayment))
	mux.Handle("POST /admin/api/settings/payment/stripe/test", owner(h.testStripeConnection))
	mux.Handle("GET /admin/api/settings/shop", owner(h.getShop))
	mux.Handle("PUT /admin/api/settings/shop", owner(h.setShop))
	mux.Handle("POST /admin/api/settings/shop/logo", owner(h.uploadShopLogo))
	mux.Handle("DELETE /admin/api/settings/shop/logo", owner(h.deleteShopLogo))
	mux.Handle("GET /admin/api/settings/policy-profile", owner(h.getPolicyProfile))
	mux.Handle("PUT /admin/api/settings/policy-profile", owner(h.setPolicyProfile))
	mux.Handle("GET /admin/api/settings/translation", owner(h.getTranslationSettings))
	mux.Handle("PUT /admin/api/settings/translation", owner(h.setTranslationSettings))
	mux.Handle("POST /admin/api/translation/text", owner(h.translateText))

	// 向导：收款步骤状态 / 跳过。
	mux.Handle("GET /admin/api/wizard/payment", protect(h.wizardPaymentStatus))
	mux.Handle("POST /admin/api/wizard/payment/skip", owner(h.wizardPaymentSkip))

	// 域名设置（写 settings.domain；env 覆盖时只读）+ 向导域名步骤（M4.2.1）。
	mux.Handle("GET /admin/api/settings/domain", owner(h.getDomain))
	mux.Handle("PUT /admin/api/settings/domain", owner(h.setDomain))
	mux.Handle("GET /admin/api/wizard/domain", protect(h.wizardDomainStatus))
	mux.Handle("POST /admin/api/wizard/domain/skip", owner(h.wizardDomainSkip))

	// SMTP 设置（password 加密存；env 覆盖时只读）+ 测试发信 + 向导邮件步骤（M4.3）。
	mux.Handle("GET /admin/api/settings/smtp", owner(h.getSMTP))
	mux.Handle("PUT /admin/api/settings/smtp", owner(h.setSMTP))
	mux.Handle("POST /admin/api/smtp/test", owner(h.smtpTest))
	mux.Handle("GET /admin/api/wizard/smtp", protect(h.wizardSMTPStatus))
	mux.Handle("POST /admin/api/wizard/smtp/skip", owner(h.wizardSMTPSkip))

	// 概览首页（登录后默认落点，M4.2.2）。
	mux.Handle("GET /admin/api/dashboard", protect(h.dashboard))
	mux.Handle("GET /admin/api/diagnostics", owner(h.diagnostics))
	mux.Handle("GET /admin/api/settings/backup", owner(h.getBackupSettings))
	mux.Handle("PUT /admin/api/settings/backup", owner(h.setBackupSettings))
	mux.Handle("POST /admin/api/settings/backup/test", owner(h.testBackupRemote))
	mux.Handle("GET /admin/api/export", owner(h.exportData))
	mux.Handle("GET /admin/api/audit-events", owner(h.listAuditEvents))

	// 订单 + 退款（M3.3a）。
	mux.Handle("GET /admin/api/orders", owner(h.listOrders))
	mux.Handle("GET /admin/api/orders/export", owner(h.exportOrdersCSV))
	mux.Handle("GET /admin/api/orders/{id}", owner(h.getOrder))
	mux.Handle("POST /admin/api/orders/{id}/refund", owner(h.refundOrder))
	mux.Handle("POST /admin/api/orders/{id}/fulfill", owner(h.fulfillOrder))
	mux.Handle("GET /admin/api/settings/shipping/countries", owner(h.listShippingCountries))
	mux.Handle("PUT /admin/api/settings/shipping/countries", owner(h.saveShippingCountries))
	mux.Handle("PUT /admin/api/settings/shipping/default", owner(h.saveDefaultShippingZone))
	mux.Handle("GET /admin/api/settings/shipping", owner(h.listShippingZones))
	mux.Handle("POST /admin/api/settings/shipping", owner(h.createShippingZone))
	mux.Handle("PATCH /admin/api/settings/shipping/{id}", owner(h.updateShippingZone))
	mux.Handle("DELETE /admin/api/settings/shipping/{id}", owner(h.deleteShippingZone))
}

func (h *HTTP) status(w http.ResponseWriter, r *http.Request) {
	init, err := h.svc.IsInitialized(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"initialized": init, "demo_mode": h.demo.Enabled})
}

func (h *HTTP) demoSession(w http.ResponseWriter, r *http.Request) {
	if !h.demo.Enabled {
		writeErr(w, http.StatusNotFound, "公开演示未启用")
		return
	}
	key := clientIP(r) + "|public-demo"
	if !h.limiter.allow(key) {
		writeErr(w, http.StatusTooManyRequests, "演示会话创建过于频繁，请稍后再试")
		return
	}
	sess, err := h.svc.CreateDemoSession(r.Context(), h.demo.SessionTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	h.setCookie(w, r, sessionCookie, sess.Token, sess.ExpiresAt, true)
	h.setCookie(w, r, csrfCookie, sess.CSRFToken, sess.ExpiresAt, false)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "expires_at": sess.ExpiresAt})
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

	h.setCookie(w, r, sessionCookie, sess.Token, sess.ExpiresAt, true)
	h.setCookie(w, r, csrfCookie, sess.CSRFToken, sess.ExpiresAt, false) // 非 HttpOnly，供 SPA 读取回传
	h.recordAudit(r, sess.AdminID, "admin.login", "admin", sess.AdminPublicID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	if ac.Role == "demo" {
		if _, err := h.cleanupDemoSession(r.Context(), ac.SessionToken); err != nil {
			writeErr(w, http.StatusInternalServerError, "清理演示数据失败")
			return
		}
	}
	if err := h.svc.Logout(r.Context(), ac.SessionToken); err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	h.clearCookie(w, r, sessionCookie, true)
	h.clearCookie(w, r, csrfCookie, false)
	h.recordAudit(r, ac.AdminID, "admin.logout", "admin", ac.AdminPublicID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// recordAudit 以最小字段记录成功动作。审计存储不可用不应把已经完成的业务操作伪装成失败，故仅记服务端错误日志。
func (h *HTTP) recordAudit(r *http.Request, adminID int64, action, targetType, targetPublicID string) {
	if h.audit == nil {
		return
	}
	if ac := authFrom(r.Context()); ac != nil && ac.Role == "demo" {
		return
	}
	if err := h.audit.Record(r.Context(), adminID, action, targetType, targetPublicID); err != nil {
		slog.Error("审计事件写入失败", "action", action, "error", err)
	}
}

func (h *HTTP) me(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"username": ac.Username, "public_id": ac.AdminPublicID, "role": ac.Role, "demo_mode": h.demo.Enabled, "expires_at": ac.ExpiresAt, "demo_max_products": h.demo.MaxProducts, "demo_max_image_bytes": h.demo.MaxImageBytes})
}

// secureFor 判定当前请求是否该发 Secure Cookie。
// 先按请求 TLS 判定；若无 TLS，仅当来源 IP 在可信代理白名单且 X-Forwarded-Proto 为 https 时采信。
func (h *HTTP) secureFor(r *http.Request) bool {
	return httpx.IsSecureRequest(r, h.trusted)
}

func (h *HTTP) setCookie(w http.ResponseWriter, r *http.Request, name, value string, expires time.Time, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", Expires: expires,
		HttpOnly: httpOnly, Secure: h.secureFor(r), SameSite: http.SameSiteLaxMode,
	})
}

func (h *HTTP) clearCookie(w http.ResponseWriter, r *http.Request, name string, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: httpOnly, Secure: h.secureFor(r), SameSite: http.SameSiteLaxMode,
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
