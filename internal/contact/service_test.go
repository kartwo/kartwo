// 客服留言测试 / Contact Service Tests
// 功能：验证默认关闭、内容校验、处理状态及到期匿名化
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 15:20:00
package contact

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"
	_ "modernc.org/sqlite"
)

func testService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/contact.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err = migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return New(db, settings.New(db)), db
}

func TestContactLifecycleAndRetention(t *testing.T) {
	s, db := testService(t)
	ctx := context.Background()
	c, err := s.Config(ctx)
	if err != nil || c.Enabled || c.RetentionMonths != 12 || c.IPRetentionDays != 90 {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	in := Input{Name: "Alice", Email: "alice@example.com", Topic: "order", Body: "Where is my order?", OrderRef: "ORD-1", IP: "203.0.113.9", Country: "US"}
	if _, err := s.Submit(ctx, in); !errors.Is(err, ErrDisabled) {
		t.Fatalf("default off: %v", err)
	}
	c.Enabled = true
	if err := s.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	invalid := in
	invalid.Email = "Alice <alice@example.com>"
	if _, err := s.Submit(ctx, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("email validation: %v", err)
	}
	id, err := s.Submit(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus(ctx, id, "closed"); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get(ctx, id)
	if err != nil || m.Status != "closed" || m.IP != in.IP {
		t.Fatalf("closed: %+v %v", m, err)
	}
	old := time.Now().UTC().AddDate(-2, 0, 0).Format(timeLayout)
	if _, err := db.ExecContext(ctx, `UPDATE contact_message SET created_at=?,closed_at=? WHERE public_id=?`, old, old, id); err != nil {
		t.Fatal(err)
	}
	if err := s.AnonymizeDue(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	m, err = s.Get(ctx, id)
	if err != nil || m.Name != "" || m.Email != "" || m.Body != "" || m.OrderRef != "" || m.IP != "" || m.Country != "" || m.AnonymizedAt == "" || m.Status != "closed" || m.Topic != "order" {
		t.Fatalf("anonymized: %+v %v", m, err)
	}
	if err := s.SetStatus(ctx, id, "new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("anonymized immutable: %v", err)
	}
	list, err := s.List(ctx, "", 100)
	if err != nil || len(list) != 1 {
		t.Fatalf("metadata kept: %d %v", len(list), err)
	}
}

func TestContactIPExpiresBeforeOpenCase(t *testing.T) {
	s, db := testService(t)
	ctx := context.Background()
	c := DefaultConfig()
	c.Enabled = true
	if err := s.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	id, err := s.Submit(ctx, Input{Name: "Bob", Email: "bob@example.com", Topic: "other", Body: "Please contact me", IP: "203.0.113.8"})
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -91).Format(timeLayout)
	if _, err := db.ExecContext(ctx, `UPDATE contact_message SET created_at=? WHERE public_id=?`, old, id); err != nil {
		t.Fatal(err)
	}
	if err := s.AnonymizeDue(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get(ctx, id)
	if err != nil || m.IP != "" || m.Name != "Bob" || m.Status != "new" {
		t.Fatalf("ip-only expiry: %+v %v", m, err)
	}
}

func TestContactTextCannotInjectSQL(t *testing.T) {
	s, db := testService(t)
	ctx := context.Background()
	c := DefaultConfig()
	c.Enabled = true
	if err := s.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	name := "Robert'); DROP TABLE contact_message; --"
	id, err := s.Submit(ctx, Input{Name: name, Email: "robert@example.com", Topic: "other", Body: "<script>alert(1)</script>"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Get(ctx, id)
	if err != nil || m.Name != name {
		t.Fatalf("text roundtrip: %+v %v", m, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM contact_message`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("table must remain: %d %v", count, err)
	}
}
