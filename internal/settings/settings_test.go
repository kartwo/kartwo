// 设置服务测试 / Settings Tests
// 功能：明文/加密读写、市场选择校验、货币派生
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-19 21:22:05
package settings

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func newSvc(t *testing.T) *Service {
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
	return New(db)
}

func TestPlainSetGet(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	if _, err := s.Get(ctx, "x"); err != ErrNotFound {
		t.Fatalf("缺项应 ErrNotFound: %v", err)
	}
	_ = s.SetPlain(ctx, "x", "hello")
	if v, _ := s.Get(ctx, "x"); v != "hello" {
		t.Fatalf("读取 = %q", v)
	}
}

func TestEncryptedSetGet(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	secret := []byte("sk_test_123")

	if err := s.SetEncrypted(ctx, "pay.key", secret, kek); err != nil {
		t.Fatal(err)
	}
	// 落库的是密文，不是明文。
	raw, _ := s.Get(ctx, "pay.key")
	if bytes.Contains([]byte(raw), secret) {
		t.Fatal("落库不应含明文")
	}
	got, err := s.GetEncrypted(ctx, "pay.key", kek)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("解密读取不符: %v / %q", err, got)
	}
	// 明文项当加密项读应报错。
	_ = s.SetPlain(ctx, "plain", "v")
	if _, err := s.GetEncrypted(ctx, "plain", kek); err != ErrNotEncrypted {
		t.Fatalf("明文项按加密读应 ErrNotEncrypted: %v", err)
	}
}

func TestMarketSelection(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	// 默认美国。
	if s.MarketCode(ctx) != "US" {
		t.Fatalf("默认市场应 US，得 %s", s.MarketCode(ctx))
	}
	if s.Currency(ctx) != "USD" {
		t.Fatalf("默认货币应 USD，得 %s", s.Currency(ctx))
	}
	// 选即将上线市场被拒。
	if err := s.SetMarketCode(ctx, "MENA"); err != ErrMarketUnavailable {
		t.Fatalf("即将上线市场应被拒: %v", err)
	}
	// 选美国成功。
	if err := s.SetMarketCode(ctx, "US"); err != nil {
		t.Fatalf("选美国应成功: %v", err)
	}
	if v, _ := s.Get(ctx, "market.code"); v != "US" {
		t.Fatalf("market.code 应已写入")
	}
}
