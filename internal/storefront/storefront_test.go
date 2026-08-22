// 店面测试 / Storefront Tests
// 功能：目录/详情组装、仅 active 可见、SEO(canonical/JSON-LD)、sitemap/robots、404
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 11:20:00
package storefront

import (
	"context"
	cryptotls "crypto/tls"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

// fakeGateway 是 PaymentGateway 的测试替身。
type fakeGateway struct {
	methods []string
	url     string
}

func (f fakeGateway) AvailableMethods(context.Context) []string { return f.methods }
func (f fakeGateway) StartCheckout(context.Context, string, payment.OrderForPayment) (string, error) {
	return f.url, nil
}
func (f fakeGateway) CapturePayPal(context.Context, string) (string, error) { return "", nil }

func TestOrderPayPendingOnly(t *testing.T) {
	sf, _, db := setup(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO customer (public_id,email,name) VALUES ('c1','a@b.com','A')`); err != nil {
		t.Fatal(err)
	}
	var cid int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM customer WHERE public_id='c1'`).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO "order" (public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) VALUES ('ORD1',?,'pending','a@b.com','A','addr','USD',9900,9900)`, cid); err != nil {
		t.Fatal(err)
	}
	h := NewHTTP(sf, cart.New(db), order.New(db, settings.New(db)), settings.New(db),
		fakeGateway{methods: []string{"stripe"}, url: "https://gw/pay"}, redirect.New(db), "Shop", "https://shop")
	mux := http.NewServeMux()
	h.Register(mux)

	pay := func() (int, string) {
		req := httptest.NewRequest("POST", "/order/ORD1/pay", strings.NewReader("payment_method=stripe"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Header().Get("Location")
	}

	// pending → 跳网关。
	if code, loc := pay(); code != http.StatusSeeOther || loc != "https://gw/pay" {
		t.Fatalf("pending 应跳网关，得 %d %s", code, loc)
	}
	// 订单页应渲染「Pay now」。
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/order/ORD1", nil))
	if !strings.Contains(rec.Body.String(), "Pay now") {
		t.Fatal("未付订单页应有 Pay now 按钮")
	}
	// 标记已付 → 去支付被拒（跳回订单页，不重复收款）。
	if _, err := db.ExecContext(ctx, `UPDATE "order" SET status='paid' WHERE public_id='ORD1'`); err != nil {
		t.Fatal(err)
	}
	if code, loc := pay(); code != http.StatusSeeOther || loc != "/order/ORD1" {
		t.Fatalf("已付不应再支付，应跳订单页，得 %d %s", code, loc)
	}
}

func setup(t *testing.T) (*Service, *catalog.Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return New(db), catalog.New(db), db
}

func activeTee(slug string) catalog.ProductInput {
	return catalog.ProductInput{
		Title: "T恤", Slug: slug, Status: "active",
		Options: []catalog.OptionInput{{Name: "尺码", Values: []string{"S", "M"}}},
		Variants: []catalog.VariantInput{
			{PriceCents: 9900, Quantity: 5, Selections: []catalog.Selection{{Option: "尺码", Value: "S"}}},
			{PriceCents: 12900, Quantity: 0, Selections: []catalog.Selection{{Option: "尺码", Value: "M"}}},
		},
	}
}

func TestListCatalog_OnlyActive(t *testing.T) {
	sf, cat, _ := setup(t)
	ctx := context.Background()
	if _, err := cat.CreateProduct(ctx, activeTee("tee")); err != nil {
		t.Fatal(err)
	}
	draft := activeTee("draft-tee")
	draft.Status = "draft"
	if _, err := cat.CreateProduct(ctx, draft); err != nil {
		t.Fatal(err)
	}

	items, err := sf.ListCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("仅 active 应可见，得 %d", len(items))
	}
	if items[0].FromCents != 9900 {
		t.Fatalf("起价 = %d，期望 9900", items[0].FromCents)
	}
}

func TestGetProduct_AssemblesDetail(t *testing.T) {
	sf, cat, _ := setup(t)
	ctx := context.Background()
	if _, err := cat.CreateProduct(ctx, activeTee("tee")); err != nil {
		t.Fatal(err)
	}
	p, err := sf.GetProduct(ctx, "tee")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Variants) != 2 {
		t.Fatalf("变体数 = %d", len(p.Variants))
	}
	if p.MinCents != 9900 || p.MaxCents != 12900 {
		t.Fatalf("价区间 = %d-%d", p.MinCents, p.MaxCents)
	}
	if !p.InStock {
		t.Fatal("应有货（S 库存 5）")
	}

	if _, err := sf.GetProduct(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("不存在应 ErrNotFound: %v", err)
	}
}

func newHTTP(t *testing.T) (*HTTP, http.Handler) {
	sf, cat, db := setup(t)
	if _, err := cat.CreateProduct(context.Background(), activeTee("tee")); err != nil {
		t.Fatal(err)
	}
	h := NewHTTP(sf, cart.New(db), order.New(db, settings.New(db)), settings.New(db), nil, redirect.New(db), "测试店", "https://shop.example")
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func get(t *testing.T, mux http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	b := rec.Body.String()
	return res.StatusCode, b
}

func TestHTTPHomeAndProduct(t *testing.T) {
	_, mux := newHTTP(t)

	code, body := get(t, mux, "/")
	if code != 200 || !strings.Contains(body, "/p/tee") || !strings.Contains(body, "测试店") {
		t.Fatalf("首页异常 code=%d", code)
	}

	code, body = get(t, mux, "/p/tee")
	if code != 200 {
		t.Fatalf("详情 code=%d", code)
	}
	for _, want := range []string{
		`<link rel="canonical" href="https://shop.example/p/tee" />`,
		`application/ld+json`,
		`"@type":"Product"`,
		`"availability":"https://schema.org/InStock"`,
		`og:type" content="product"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("详情缺少 SEO 片段: %s", want)
		}
	}

	if code, _ := get(t, mux, "/p/missing"); code != http.StatusNotFound {
		t.Fatalf("缺商品应 404，得 %d", code)
	}
}

func TestHTTPShopifyProductRedirect(t *testing.T) {
	sf, cat, db := setup(t)
	ctx := context.Background()
	publicID, err := cat.CreateProduct(ctx, activeTee("new-tee"))
	if err != nil {
		t.Fatal(err)
	}
	productID, err := cat.ProductIDByPublicID(ctx, publicID)
	if err != nil {
		t.Fatal(err)
	}
	redirectSvc := redirect.New(db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := redirectSvc.StoreShopifyTx(ctx, tx, "old-tee", productID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	h := NewHTTP(sf, cart.New(db), order.New(db, settings.New(db)), settings.New(db), nil, redirectSvc, "测试店", "https://shop.example")
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/products/old-tee?utm_source=shopify", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/p/new-tee?utm_source=shopify" {
		t.Fatalf("旧链接应 301 到新地址，code=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if code, _ := get(t, mux, "/products/missing"); code != http.StatusNotFound {
		t.Fatalf("未知旧链接应 404，得 %d", code)
	}
	if _, err := db.ExecContext(ctx, `UPDATE product SET status='draft' WHERE id=?`, productID); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, mux, "/products/old-tee"); code != http.StatusNotFound {
		t.Fatalf("草稿商品旧链接应 404，得 %d", code)
	}
}

func TestHTTPSitemapRobots(t *testing.T) {
	_, mux := newHTTP(t)
	code, body := get(t, mux, "/sitemap.xml")
	if code != 200 || !strings.Contains(body, "https://shop.example/p/tee") {
		t.Fatalf("sitemap 异常 code=%d", code)
	}
	code, body = get(t, mux, "/robots.txt")
	if code != 200 || !strings.Contains(body, "Sitemap: https://shop.example/sitemap.xml") || !strings.Contains(body, "Disallow: /admin/") {
		t.Fatalf("robots 异常 code=%d body=%s", code, body)
	}
}

// JSON-LD 必须是合法可解析 JSON。
func TestJSONLDValid(t *testing.T) {
	_, mux := newHTTP(t)
	_, body := get(t, mux, "/p/tee")
	start := strings.Index(body, `application/ld+json">`)
	if start < 0 {
		t.Fatal("无 JSON-LD")
	}
	start += len(`application/ld+json">`)
	end := strings.Index(body[start:], "</script>")
	raw := body[start : start+end]
	if !json.Valid([]byte(raw)) {
		t.Fatalf("JSON-LD 非法: %s", raw)
	}
}

// TestCartCookieSecureFollowsRequestTLS 锁死第三处 cookie（购物车）与 D8-A 同口径：
// Secure 按**请求实际是否走 TLS**决定，而非静态 Env=="prod"。设置与清除两条路径各测两分支。
//
// 机理：明文来源发出的带 Secure 的 Set-Cookie 会被浏览器整条丢弃（RFC 6265bis §5.5）。
// 对购物车 cookie 而言，prod 明文态（HTTP-only 评估态 / 裸 IP 逃生路）下顾客每次请求
// 都拿到新的空车 —— 加购不生效。清除路径同理：那条 Max-Age=-1 根本不会被处理。
func TestCartCookieSecureFollowsRequestTLS(t *testing.T) {
	// cartSet 触发 cartCtx 下发新车 cookie；返回该响应里的购物车 cookie。
	cartCookieFrom := func(t *testing.T, mux http.Handler, tls bool) *http.Cookie {
		t.Helper()
		req := httptest.NewRequest("GET", "/cart", nil)
		if tls {
			req.TLS = &cryptotls.ConnectionState{} // httptest 默认 r.TLS==nil
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		res := rec.Result()
		defer func() { _ = res.Body.Close() }()
		for _, c := range res.Cookies() {
			if c.Name == cartCookie {
				return c
			}
		}
		t.Fatalf("应下发购物车 cookie，得 %v", res.Cookies())
		return nil
	}

	t.Run("设置路径", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			tls  bool
			want bool
		}{
			{"TLS 请求 → 带 Secure", true, true},
			{"明文请求 → 不带 Secure（否则顾客加购不生效）", false, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, mux := newHTTP(t)
				c := cartCookieFrom(t, mux, tc.tls)
				if c.Secure != tc.want {
					t.Fatalf("Secure=%v，期望 %v", c.Secure, tc.want)
				}
				// 其余属性不受影响。
				if !c.HttpOnly {
					t.Fatal("购物车 cookie 必须保持 HttpOnly")
				}
				if c.SameSite != http.SameSiteLaxMode {
					t.Fatalf("SameSite 应保持 Lax，得 %v", c.SameSite)
				}
			})
		}
	})

	t.Run("清除路径", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			tls  bool
			want bool
		}{
			{"TLS 请求 → 清除指令带 Secure", true, true},
			{"明文请求 → 清除指令不带 Secure", false, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				h := &HTTP{} // clearCartCookie 只用 w/r，不碰其它字段
				req := httptest.NewRequest("POST", "/checkout", nil)
				if tc.tls {
					req.TLS = &cryptotls.ConnectionState{}
				}
				rec := httptest.NewRecorder()
				h.clearCartCookie(rec, req)
				res := rec.Result()
				defer func() { _ = res.Body.Close() }()
				var got *http.Cookie
				for _, c := range res.Cookies() {
					if c.Name == cartCookie {
						got = c
					}
				}
				if got == nil {
					t.Fatal("应下发购物车清除指令")
				}
				if got.MaxAge >= 0 {
					t.Fatalf("清除指令 MaxAge 应为负，得 %d", got.MaxAge)
				}
				if got.Secure != tc.want {
					t.Fatalf("清除指令 Secure=%v，期望 %v", got.Secure, tc.want)
				}
			})
		}
	})
}
