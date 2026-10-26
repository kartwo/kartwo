// 后台入口测试装配 / Admin Entry Test Fixture
// 功能：独立文件数据库与正式媒体/导入/导出/店面和安全中间件，无外部工作进程
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 12:23:58
package server

import (
	"context"
	"github.com/kartwo/kartwo/internal/admin"
	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/importer"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store"
	"github.com/kartwo/kartwo/internal/storefront"
	"github.com/kartwo/kartwo/migrations"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func newAdminEntryFixture(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	return openAdminEntryFixture(t, t.TempDir())
}
func openAdminEntryFixture(t *testing.T, dir string) (http.Handler, *store.Store, string) {
	t.Helper()
	st, e := store.Open(&config.Config{DBEngine: "sqlite", DataDir: dir, DBPath: filepath.Join(dir, "shop.db")})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, e = migrate.Run(context.Background(), st.DB, migrations.FS); e != nil {
		t.Fatal(e)
	}
	set := settings.New(st.DB)
	svc := admin.New(st.DB)
	keys := payment.NewKeyCache(set)
	svc.SetPaymentKeys(keys)
	cache := mail.NewCache(set)
	svc.SetMailKeys(cache)
	root := filepath.Join(dir, "media")
	md := media.New(st.DB, media.NewLocalBackend(root), media.NewDefaultPolicy(root, 10<<20, 0), 20)
	cat := catalog.New(st.DB)
	rd := redirect.New(st.DB)
	h := admin.NewHTTP(svc, cat, importer.New(st.DB, cat, md, rd), md, set, order.New(st.DB, set), nil, cache, backup.New(st.DB, dir, "entry-flow-test"), admin.BackupConfig{}, "", false, nil)
	h.ConfigureDemo(admin.DemoConfig{Enabled: true, SessionTTL: time.Hour, MaxProducts: 3})
	customer := storefront.NewHTTP(storefront.New(st.DB), cart.New(st.DB), order.New(st.DB, set), set, nil, rd, "Entry fixture", "https://entry.example.invalid", nil)
	handler := New(&config.Config{DataDir: dir}, st, "entry-flow-test", h, customer, payment.NewHTTP(payment.NewService(st.DB, set, keys)), false)
	return handler, st, root
}
