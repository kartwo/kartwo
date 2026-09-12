// 店面 HTTP / Storefront HTTP (SSR)
// 功能：服务端渲染目录/详情页 + SEO(canonical/OG/JSON-LD) + sitemap.xml + robots.txt
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 11:20:00
package storefront

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"

	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/httpx"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/settings"
)

// PaymentGateway 收款网关（由 internal/payment 实现）。nil 表示未接入收款，结算退化为「下单未付」。
type PaymentGateway interface {
	AvailableMethods(ctx context.Context) []string
	StartCheckout(ctx context.Context, provider string, ord payment.OrderForPayment) (string, error)
	CapturePayPal(ctx context.Context, paypalOrderID string) (string, error)
}

//go:embed templates/*.html static/*
var tmplFS embed.FS

// HTTP 承载店面页面与 SEO 端点。
type HTTP struct {
	trusted          []*net.IPNet
	svc              *Service
	cart             *cart.Service
	order            *order.Service
	settings         *settings.Service
	pay              PaymentGateway // 可为 nil（未接入收款）
	redirect         *redirect.Service
	shopNameFallback string
	baseURL          string // 配置基址；空则按请求推导
	homeTmpl         *template.Template
	catalogTmpl      *template.Template
	prodTmpl         *template.Template
	cartTmpl         *template.Template
	ckoutTmpl        *template.Template
	orderTmpl        *template.Template
	pageTmpl         *template.Template
}

// secureFor 判定本次响应的 Cookie 是否该带 Secure：直连 TLS 始终安全；
// 反向代理终止 TLS 时，仅可信代理白名单内的 X-Forwarded-Proto=https 可被采信。
//
// 明文来源发出的带 Secure 的 Set-Cookie 会被浏览器整条丢弃（RFC 6265bis §5.5）——
// 对购物车 cookie 意味着 prod 明文态（HTTP-only 评估态 / 裸 IP 逃生路）下
// 顾客每次请求都拿到新的空车，**加购不生效**。
func (h *HTTP) secureFor(r *http.Request) bool {
	return httpx.IsSecureRequest(r, h.trusted)
}

// NewHTTP 构建店面 HTTP 层。货币按当前主攻市场逐请求解析（向导切市场即时生效）。
// 注：cookie 的 Secure 标记按**每次请求**是否走 TLS 决定（见 secureFor），故不再需要 secure 参数。
func NewHTTP(svc *Service, cartSvc *cart.Service, orderSvc *order.Service, settingsSvc *settings.Service, pay PaymentGateway, redirectSvc *redirect.Service, shopName, baseURL string, trusted []*net.IPNet, shopNameOverride ...string) *HTTP {
	parse := func(page string) *template.Template {
		return template.Must(template.New("").ParseFS(tmplFS, "templates/base.html", page))
	}
	h := &HTTP{
		trusted: trusted,
		svc:     svc, cart: cartSvc, order: orderSvc, settings: settingsSvc, pay: pay, redirect: redirectSvc, shopNameFallback: shopName,
		baseURL:     strings.TrimRight(baseURL, "/"),
		homeTmpl:    parse("templates/home.html"),
		catalogTmpl: parse("templates/catalog.html"),
		prodTmpl:    parse("templates/product.html"),
		cartTmpl:    parse("templates/cart.html"),
		ckoutTmpl:   parse("templates/checkout.html"),
		orderTmpl:   parse("templates/order.html"),
		pageTmpl:    parse("templates/content_page.html"),
	}
	if len(shopNameOverride) > 0 {
		if fallback := strings.TrimSpace(shopNameOverride[0]); fallback != "" {
			h.shopNameFallback = fallback
		}
	}
	return h
}

// cur 解析当前请求的货币代码（按主攻市场）。
func (h *HTTP) cur(ctx context.Context) string { return h.settings.Currency(ctx) }

// shopName 返回当前有效店名；后台设置始终可修改，环境变量仅作为未配置时的回退。
func (h *HTTP) shopName(ctx context.Context) string {
	return h.settings.ShopName(ctx, h.shopNameFallback)
}

// money 返回当前请求的金额格式化器（供模板 {{call $.Money .Cents}}）。
func (h *HTTP) money(ctx context.Context) func(int64) string { return moneyFunc(h.cur(ctx)) }

// Register 注册店面路由（公开，无鉴权）。
func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.home) // 仅根路径，避免吃掉其它前缀
	mux.HandleFunc("GET /p/{slug}", h.product)
	mux.HandleFunc("GET /collections/all", h.allProducts)
	mux.HandleFunc("GET /collections/{slug}", h.categoryPage)
	mux.HandleFunc("GET /search", h.searchPage)
	mux.HandleFunc("GET /pages/{slug}", h.contentPage)
	mux.HandleFunc("GET /products/{handle}", h.shopifyProductRedirect)
	mux.HandleFunc("GET /sitemap.xml", h.sitemap)
	mux.HandleFunc("GET /robots.txt", h.robots)
	mux.HandleFunc("GET /static/cart.js", h.cartJS)
	mux.HandleFunc("GET /static/running-tee-hero.png", h.runningTeeHero)
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
	mux.HandleFunc("POST /order/{id}/pay", h.orderPay)   // 未付订单「去支付」重新发起
	mux.HandleFunc("GET /paypal/return", h.paypalReturn) // PayPal 审批后跳回做同步 capture
}

// shopifyProductRedirect 将 Shopify 历史商品链接永久迁移到 Kartwo 商品页。
func (h *HTTP) shopifyProductRedirect(w http.ResponseWriter, r *http.Request) {
	if h.redirect == nil {
		http.NotFound(w, r)
		return
	}
	slug, err := h.redirect.ResolveShopifyHandle(r.Context(), r.PathValue("handle"))
	if errors.Is(err, redirect.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	target := "/p/" + slug
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
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
	if h.secureFor(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *HTTP) home(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListFeaturedCatalog(r.Context())
	if err == nil && len(items) == 0 {
		items, err = h.svc.ListCatalog(r.Context())
	}
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	canonical := h.base(r) + "/"
	shopName := h.shopName(r.Context())
	ld := map[string]any{
		"@context": "https://schema.org", "@type": "WebSite", "name": shopName, "url": canonical,
	}
	data := map[string]any{
		"ShopName": shopName,
		"Items":    items,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title: shopName + " — Shop", Description: shopName + " catalog",
			Canonical: canonical, OGType: "website", JSONLD: jsonLD(ld),
		},
	}
	h.render(w, r, h.homeTmpl, data)
}

func (h *HTTP) allProducts(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListCatalog(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	shopName := h.shopName(r.Context())
	h.render(w, r, h.catalogTmpl, map[string]any{
		"ShopName": shopName, "Heading": "All products", "Intro": "Explore the complete running collection.", "Items": items, "Money": h.money(r.Context()),
		"SEO": seo{Title: "All products — " + shopName, Description: "Shop all products from " + shopName, Canonical: h.base(r) + "/collections/all", OGType: "website"},
	})
}

func (h *HTTP) categoryPage(w http.ResponseWriter, r *http.Request) {
	category, items, err := h.svc.ListCatalogByCategory(r.Context(), r.PathValue("slug"))
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	shopName := h.shopName(r.Context())
	h.render(w, r, h.catalogTmpl, map[string]any{
		"ShopName": shopName, "Heading": category.Name, "Intro": fmt.Sprintf("%d products in this collection.", category.ProductCount), "Items": items, "Money": h.money(r.Context()),
		"SEO": seo{Title: category.Name + " — " + shopName, Description: "Shop " + category.Name + " from " + shopName, Canonical: h.base(r) + "/collections/" + category.Slug, OGType: "website"},
	})
}

func (h *HTTP) searchPage(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	if term == "" {
		http.Redirect(w, r, "/collections/all", http.StatusSeeOther)
		return
	}
	if len(term) > 100 {
		term = term[:100]
	}
	items, err := h.svc.SearchCatalog(r.Context(), term)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	shopName := h.shopName(r.Context())
	h.render(w, r, h.catalogTmpl, map[string]any{
		"ShopName": shopName, "Heading": "Search results", "Intro": fmt.Sprintf("%d results for “%s”.", len(items), term), "SearchTerm": term, "Items": items, "Money": h.money(r.Context()),
		"SEO": seo{Title: "Search — " + shopName, Description: "Product search results", Canonical: h.base(r) + "/search", OGType: "website"},
	})
}

func (h *HTTP) contentPage(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetContentPage(r.Context(), r.PathValue("slug"))
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	shopName := h.shopName(r.Context())
	canonical := h.base(r) + "/pages/" + p.Slug
	h.render(w, r, h.pageTmpl, map[string]any{"ShopName": shopName, "Page": p, "Body": safeMarkdown(p.BodyMarkdown), "SEO": seo{Title: p.Title + " — " + shopName, Description: seoDescription(p.SEODescription, p.Title), Canonical: canonical, OGType: "article"}})
}

// safeMarkdown 只支持标题、段落、无序列表和行内强调；所有输入先转义，故内容页不能注入 HTML/脚本。
func safeMarkdown(src string) template.HTML {
	var b strings.Builder
	inList := false
	closeList := func() {
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			closeList()
			continue
		}
		esc := template.HTMLEscapeString(line)
		if strings.HasPrefix(esc, "### ") {
			closeList()
			b.WriteString("<h3>" + esc[4:] + "</h3>")
			continue
		}
		if strings.HasPrefix(esc, "## ") {
			closeList()
			b.WriteString("<h2>" + esc[3:] + "</h2>")
			continue
		}
		if strings.HasPrefix(esc, "# ") {
			closeList()
			b.WriteString("<h1>" + esc[2:] + "</h1>")
			continue
		}
		if strings.HasPrefix(esc, "- ") {
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + esc[2:] + "</li>")
			continue
		}
		closeList()
		b.WriteString("<p>" + esc + "</p>")
	}
	closeList()
	return template.HTML(b.String()) //nolint:gosec // every input line is escaped before only fixed safe tags are added
}

func (h *HTTP) product(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	p, err := h.svc.GetProduct(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	canonical := h.base(r) + "/p/" + p.Slug
	var ogImage string
	if len(p.Images) > 0 {
		ogImage = h.base(r) + p.Images[0].Large
	}
	shopName := h.shopName(r.Context())
	data := map[string]any{
		"ShopName": shopName,
		"Product":  p,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title:       p.Title + " — " + shopName,
			Description: seoDescription(firstNonEmpty(p.SEODescription, p.Description), p.Title),
			Canonical:   canonical, OGType: "product", OGImage: ogImage,
			JSONLD: jsonLD(h.productLD(r.Context(), p, canonical, ogImage)),
		},
	}
	h.render(w, r, h.prodTmpl, data)
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
		"name": p.Title, "description": seoDescription(firstNonEmpty(p.SEODescription, p.Description), p.Title),
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
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
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
	categories, err := h.svc.ListCategories(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	for _, category := range categories {
		b.WriteString("  <url><loc>" + xmlEsc(base+"/collections/"+category.Slug) + "</loc></url>\n")
	}
	pages, err := h.svc.ListContentPages(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	for _, page := range pages {
		b.WriteString("  <url><loc>" + xmlEsc(base+"/pages/"+page.Slug) + "</loc></url>\n")
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (h *HTTP) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /admin/\nSitemap: %s/sitemap.xml\n", h.base(r))
}

// runningTeeHero 提供内嵌的原创跑步产品首屏图；固定路径，不接受用户输入。
func (h *HTTP) runningTeeHero(w http.ResponseWriter, _ *http.Request) {
	b, err := tmplFS.ReadFile("static/running-tee-hero.png")
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = w.Write(b)
}

func (h *HTTP) render(w http.ResponseWriter, r *http.Request, t *template.Template, data map[string]any) {
	categories, err := h.svc.ListCategories(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	pages, err := h.svc.ListContentPages(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	data["NavigationCategories"] = categories
	data["FooterPages"] = pages
	data["ShopLogoURL"] = h.settings.ShopLogoURL(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "Render error", http.StatusInternalServerError)
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
