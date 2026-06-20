// 店面测试 / Storefront Tests
// 功能：目录/详情组装、仅 active 可见、SEO(canonical/JSON-LD)、sitemap/robots、404
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 11:20:00
package storefront

import (
	"context"
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
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

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
	h := NewHTTP(sf, cart.New(db), order.New(db, settings.New(db)), settings.New(db), nil, "测试店", "https://shop.example", false)
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
