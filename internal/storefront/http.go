// 店面 HTTP / Storefront HTTP (SSR)
// 功能：服务端渲染目录/详情页 + SEO(canonical/OG/JSON-LD) + sitemap.xml + robots.txt
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 11:20:00
package storefront

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"context"

	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/settings"
)

//go:embed templates/*.html static/*
var tmplFS embed.FS

// HTTP 承载店面页面与 SEO 端点。
type HTTP struct {
	svc       *Service
	cart      *cart.Service
	order     *order.Service
	settings  *settings.Service
	shopName  string
	baseURL   string // 配置基址；空则按请求推导
	secure    bool   // prod 下 cookie 加 Secure
	homeTmpl  *template.Template
	prodTmpl  *template.Template
	cartTmpl  *template.Template
	ckoutTmpl *template.Template
	orderTmpl *template.Template
}

// NewHTTP 构建店面 HTTP 层。货币按当前主攻市场逐请求解析（向导切市场即时生效）。
func NewHTTP(svc *Service, cartSvc *cart.Service, orderSvc *order.Service, settingsSvc *settings.Service, shopName, baseURL string, secure bool) *HTTP {
	parse := func(page string) *template.Template {
		return template.Must(template.New("").ParseFS(tmplFS, "templates/base.html", page))
	}
	return &HTTP{
		svc: svc, cart: cartSvc, order: orderSvc, settings: settingsSvc, shopName: shopName,
		baseURL: strings.TrimRight(baseURL, "/"), secure: secure,
		homeTmpl:  parse("templates/home.html"),
		prodTmpl:  parse("templates/product.html"),
		cartTmpl:  parse("templates/cart.html"),
		ckoutTmpl: parse("templates/checkout.html"),
		orderTmpl: parse("templates/order.html"),
	}
}

// cur 解析当前请求的货币代码（按主攻市场）。
func (h *HTTP) cur(ctx context.Context) string { return h.settings.Currency(ctx) }

// money 返回当前请求的金额格式化器（供模板 {{call $.Money .Cents}}）。
func (h *HTTP) money(ctx context.Context) func(int64) string { return moneyFunc(h.cur(ctx)) }

// Register 注册店面路由（公开，无鉴权）。
func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.home) // 仅根路径，避免吃掉其它前缀
	mux.HandleFunc("GET /p/{slug}", h.product)
	mux.HandleFunc("GET /sitemap.xml", h.sitemap)
	mux.HandleFunc("GET /robots.txt", h.robots)
	mux.HandleFunc("GET /static/cart.js", h.cartJS)
	// 购物车（匿名，cookie 标识；SameSite=Lax 缓解 CSRF）。
	mux.HandleFunc("GET /cart", h.cartPage)
	mux.HandleFunc("GET /cart/data", h.cartData)
	mux.HandleFunc("POST /cart/items", h.cartAdd)
	mux.HandleFunc("PATCH /cart/items/{vid}", h.cartSet)
	mux.HandleFunc("DELETE /cart/items/{vid}", h.cartRemove)
	// 结算/订单（表单提交，无 JS 也可用）。
	mux.HandleFunc("GET /checkout", h.checkoutPage)
	mux.HandleFunc("POST /checkout", h.checkoutSubmit)
	mux.HandleFunc("GET /order/{id}", h.orderPage)
}

type seo struct {
	Title       string
	Description string
	Canonical   string
	OGType      string
	OGImage     string
	JSONLD      template.HTML
}

func (h *HTTP) base(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *HTTP) home(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListCatalog(r.Context())
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	canonical := h.base(r) + "/"
	ld := map[string]any{
		"@context": "https://schema.org", "@type": "WebSite", "name": h.shopName, "url": canonical,
	}
	data := map[string]any{
		"ShopName": h.shopName,
		"Items":    items,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title: h.shopName + " — 全部商品", Description: h.shopName + " 的商品目录",
			Canonical: canonical, OGType: "website", JSONLD: jsonLD(ld),
		},
	}
	h.render(w, h.homeTmpl, data)
}

func (h *HTTP) product(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	p, err := h.svc.GetProduct(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	canonical := h.base(r) + "/p/" + p.Slug
	var ogImage string
	if len(p.Images) > 0 {
		ogImage = h.base(r) + p.Images[0].Large
	}
	data := map[string]any{
		"ShopName": h.shopName,
		"Product":  p,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title:       p.Title + " — " + h.shopName,
			Description: seoDescription(p.Description, p.Title),
			Canonical:   canonical, OGType: "product", OGImage: ogImage,
			JSONLD: jsonLD(h.productLD(r.Context(), p, canonical, ogImage)),
		},
	}
	h.render(w, h.prodTmpl, data)
}

// productLD 构建 schema.org/Product 结构化数据（含 offers 价格/库存）。
func (h *HTTP) productLD(ctx context.Context, p *ProductPage, canonical, image string) map[string]any {
	avail := "https://schema.org/OutOfStock"
	if p.InStock {
		avail = "https://schema.org/InStock"
	}
	offers := map[string]any{
		"@type": "AggregateOffer", "priceCurrency": h.cur(ctx),
		"lowPrice": cents2str(p.MinCents), "highPrice": cents2str(p.MaxCents),
		"offerCount": len(p.Variants), "availability": avail, "url": canonical,
	}
	ld := map[string]any{
		"@context": "https://schema.org", "@type": "Product",
		"name": p.Title, "description": seoDescription(p.Description, p.Title),
		"sku": p.PublicID, "offers": offers,
	}
	if image != "" {
		ld["image"] = image
	}
	return ld
}

func (h *HTTP) sitemap(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListCatalog(r.Context())
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	base := h.base(r)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	b.WriteString("  <url><loc>" + xmlEsc(base+"/") + "</loc></url>\n")
	for _, it := range items {
		b.WriteString("  <url><loc>" + xmlEsc(base+"/p/"+it.Slug) + "</loc></url>\n")
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (h *HTTP) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /admin/\nSitemap: %s/sitemap.xml\n", h.base(r))
}

func (h *HTTP) render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "渲染失败", http.StatusInternalServerError)
	}
}

// ---- helpers ----

func moneyFunc(currency string) func(int64) string {
	sym := map[string]string{"CNY": "¥", "USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥"}[currency]
	if sym == "" {
		sym = currency + " "
	}
	return func(cents int64) string { return sym + cents2str(cents) }
}

func cents2str(cents int64) string {
	neg := ""
	if cents < 0 {
		neg, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d", neg, cents/100, cents%100)
}

// jsonLD 把结构化数据序列化为完整的 <script type="application/ld+json"> 块。
// 整块作为 template.HTML 注入（绕开 html/template 在 script 上下文里把 JSON 再次转成字符串的行为）；
// 转义 < / & 防 </script> 与实体逃逸。
func jsonLD(v any) template.HTML {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(b), "<", "\\u003c")
	s = strings.ReplaceAll(s, "&", "\\u0026")
	return template.HTML(`<script type="application/ld+json">` + s + `</script>`) //nolint:gosec // 已转义 <、&；内容来自服务端 json.Marshal
}

func seoDescription(desc, fallback string) string {
	d := strings.TrimSpace(desc)
	if d == "" {
		d = fallback
	}
	if len(d) > 160 {
		d = d[:160]
	}
	return d
}

func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
