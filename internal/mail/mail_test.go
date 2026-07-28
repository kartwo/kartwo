// 邮件队列测试 / Mail Outbox & Worker Tests
// 功能：入队幂等、worker 认领/发送/退避/死信、未配置 skip、金库锁定留 pending、SMTP 配置载入(env/db)
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package mail

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	// FK off：避免为 outbox/worker 单测拼全套 product/variant 外键链。
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/m.db")
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// enqueueRaw 直接入一条 outbox（免整套订单外键），返回 id。
func enqueueRaw(t *testing.T, db *sql.DB, orderID int64, to string) int64 {
	t.Helper()
	q := sqlcgen.New(db)
	if _, err := q.EnqueueEmail(context.Background(), sqlcgen.EnqueueEmailParams{
		OrderID: orderID, Kind: KindOrderConfirmation, ToAddr: to, Subject: "s", Body: "b",
	}); err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM email_outbox WHERE order_id=?`, orderID).Scan(&id); err != nil {
		t.Fatalf("取 id 失败: %v", err)
	}
	return id
}

func rowState(t *testing.T, db *sql.DB, id int64) (status string, attempts int64) {
	t.Helper()
	if err := db.QueryRow(`SELECT status, attempts FROM email_outbox WHERE id=?`, id).Scan(&status, &attempts); err != nil {
		t.Fatalf("读行失败: %v", err)
	}
	return
}

// dbConfiguredCache 造一个已解锁、库来源、可发信的 cache（host+from 齐）。
func dbConfiguredCache(t *testing.T, db *sql.DB) *Cache {
	t.Helper()
	set := settings.New(db)
	ctx := context.Background()
	_ = set.SetPlain(ctx, KeySMTPHost, "smtp.example.com")
	_ = set.SetPlain(ctx, KeySMTPPort, "587")
	_ = set.SetPlain(ctx, KeySMTPFromAddress, "shop@example.com")
	_ = set.SetPlain(ctx, KeySMTPEncryption, "starttls")
	c := NewCache(set)
	if err := c.Unlock(ctx, make([]byte, 32)); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
	return c
}

// ---- 入队幂等 ----

func TestEnqueueOrderConfirmationIdempotent(t *testing.T) {
	db := newDB(t)
	q := sqlcgen.New(db)
	ctx := context.Background()
	// 直接插一条 order_item（FK off），供组信读取。
	if _, err := db.Exec(`INSERT INTO "order" (id,public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) VALUES (7,'019order00000007',1,'paid','buyer@demo.co','N','A','USD',9900,9900)`); err != nil {
		t.Fatalf("插订单失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO order_item (order_id,variant_id,product_title,variant_label,unit_cents,quantity,line_cents) VALUES (7,1,'T恤','尺码:M',9900,1,9900)`); err != nil {
		t.Fatalf("插订单行失败: %v", err)
	}
	o := OrderInfo{ID: 7, PublicID: "019order00000007", Email: "buyer@demo.co", Currency: "USD", TotalCents: 9900}

	for i := 0; i < 3; i++ { // 双触发（PayPal 同步 capture + webhook 备份）模拟：多次入队
		if err := EnqueueOrderConfirmation(ctx, q, o); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_outbox WHERE order_id=7`).Scan(&n); err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("同订单同信种应只入队一封，得 %d", n)
	}
	// 正文含订单号/金额/商品。
	var subject, body, to string
	_ = db.QueryRow(`SELECT subject, body, to_addr FROM email_outbox WHERE order_id=7`).Scan(&subject, &body, &to)
	if to != "buyer@demo.co" || subject == "" ||
		!contains(body, "019order00000007") || !contains(body, "USD 99.00") || !contains(body, "T恤") {
		t.Fatalf("正文/收件人不符: to=%q subject=%q body=%q", to, subject, body)
	}
}

// ---- worker：发送成功 ----

func TestWorkerSendSuccess(t *testing.T) {
	db := newDB(t)
	id := enqueueRaw(t, db, 1, "a@b.co")
	w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
	var sentTo string
	w.send = func(_ Config, to, _, _ string) error { sentTo = to; return nil }
	w.tick(context.Background())
	if st, _ := rowState(t, db, id); st != "sent" {
		t.Fatalf("应 sent，得 %s", st)
	}
	if sentTo != "a@b.co" {
		t.Fatalf("收件人应 a@b.co，得 %s", sentTo)
	}
}

// ---- worker：失败重试 + 最终死信 ----

func TestWorkerRetryThenFail(t *testing.T) {
	db := newDB(t)
	id := enqueueRaw(t, db, 1, "a@b.co")
	w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
	w.send = func(Config, string, string, string) error { return errors.New("smtp down") }

	// 第 1~4 次 tick：失败→重试（回 pending、attempts 递增、退避把 next_attempt 推到未来）。
	for i := 1; i <= 4; i++ {
		// 把 next_attempt_at 拨回过去，确保可被再次认领。
		if _, err := db.Exec(`UPDATE email_outbox SET next_attempt_at='2000-01-01T00:00:00.000Z' WHERE id=? AND status='pending'`, id); err != nil {
			t.Fatalf("重置到期失败: %v", err)
		}
		w.tick(context.Background())
		st, att := rowState(t, db, id)
		if st != "pending" || att != int64(i) {
			t.Fatalf("第 %d 次失败后应 pending/attempts=%d，得 %s/%d", i, i, st, att)
		}
	}
	// 第 5 次：达上限 → failed 死信。
	if _, err := db.Exec(`UPDATE email_outbox SET next_attempt_at='2000-01-01T00:00:00.000Z' WHERE id=?`, id); err != nil {
		t.Fatalf("重置到期失败: %v", err)
	}
	w.tick(context.Background())
	if st, att := rowState(t, db, id); st != "failed" || att != 5 {
		t.Fatalf("第 5 次应 failed/attempts=5，得 %s/%d", st, att)
	}
}

// ---- worker：SMTP 未配置 → skipped（D7-A）----

func TestWorkerUnconfiguredSkips(t *testing.T) {
	db := newDB(t)
	id := enqueueRaw(t, db, 1, "a@b.co")
	// 空 cache（无 env、无 db 配置）。
	w := NewWorker(db, NewCache(settings.New(db)), quietLogger(), time.Second)
	w.send = func(Config, string, string, string) error { t.Fatal("未配置不应发送"); return nil }
	w.tick(context.Background())
	if st, _ := rowState(t, db, id); st != "skipped" {
		t.Fatalf("未配置应 skipped，得 %s", st)
	}
}

// ---- worker：已配置但金库锁定 → 留 pending（D2）----

func TestWorkerLockedLeavesPending(t *testing.T) {
	db := newDB(t)
	id := enqueueRaw(t, db, 1, "a@b.co")
	set := settings.New(db)
	ctx := context.Background()
	// 库里存了配置，但 cache 未 Unlock（金库锁定）。
	_ = set.SetPlain(ctx, KeySMTPHost, "smtp.example.com")
	_ = set.SetPlain(ctx, KeySMTPPort, "587")
	_ = set.SetPlain(ctx, KeySMTPFromAddress, "shop@example.com")
	c := NewCache(set) // 未 Unlock
	w := NewWorker(db, c, quietLogger(), time.Second)
	w.send = func(Config, string, string, string) error { t.Fatal("锁定态不应发送"); return nil }
	w.tick(ctx)
	if st, _ := rowState(t, db, id); st != "pending" {
		t.Fatalf("锁定态应留 pending，得 %s", st)
	}
	// 解锁后应能发出。
	if err := c.Unlock(ctx, make([]byte, 32)); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
	sent := false
	w.send = func(Config, string, string, string) error { sent = true; return nil }
	w.tick(ctx)
	if st, _ := rowState(t, db, id); st != "sent" || !sent {
		t.Fatalf("解锁后应发出、sent，得 %s/%v", st, sent)
	}
}

// ---- SMTP 配置载入：env 覆盖 + db 解锁 ----

func TestCacheEnvOverride(t *testing.T) {
	t.Setenv("SMTP_HOST", "env.smtp.com")
	t.Setenv("SMTP_FROM_ADDRESS", "env@shop.co")
	t.Setenv("SMTP_PASSWORD", "envpass")
	c := NewCache(settings.New(newDB(t)))
	if !c.EnvOverride() {
		t.Fatal("应处于 env 覆盖")
	}
	cfg, ok := c.Config()
	if !ok || cfg.Host != "env.smtp.com" || cfg.FromAddress != "env@shop.co" || cfg.Password != "envpass" || cfg.Port != "587" {
		t.Fatalf("env 配置不符: %+v ok=%v", cfg, ok)
	}
	// env 模式无需 Unlock 即可用（worker 不依赖登录）。
	if st := c.Status(context.Background()); st.Source != "env" || !st.Configured {
		t.Fatalf("env 状态应 source=env configured=true: %+v", st)
	}
}

func TestCacheDBUnlockDecryptsPassword(t *testing.T) {
	db := newDB(t)
	set := settings.New(db)
	ctx := context.Background()
	kek := make([]byte, 32)
	_ = set.SetPlain(ctx, KeySMTPHost, "smtp.example.com")
	_ = set.SetPlain(ctx, KeySMTPPort, "465")
	_ = set.SetPlain(ctx, KeySMTPFromAddress, "shop@example.com")
	_ = set.SetPlain(ctx, KeySMTPEncryption, "tls")
	if err := set.SetEncrypted(ctx, KeySMTPPassword, []byte("s3cret"), kek); err != nil {
		t.Fatalf("加密存密码失败: %v", err)
	}
	// 磁盘上密码应为密文。
	if raw, _ := set.Get(ctx, KeySMTPPassword); contains(raw, "s3cret") {
		t.Fatal("磁盘不应含密码明文")
	}
	c := NewCache(set)
	if _, ok := c.Config(); ok {
		t.Fatal("未解锁前不应可发信")
	}
	if !c.Configured(ctx) {
		t.Fatal("库已存 host+from，Configured 应为 true（用于区分锁定 vs 未配置）")
	}
	if err := c.Unlock(ctx, kek); err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
	cfg, ok := c.Config()
	if !ok || cfg.Password != "s3cret" || cfg.Encryption != "tls" {
		t.Fatalf("解锁后应取回明文密码: %+v ok=%v", cfg, ok)
	}
	c.Lock()
	if _, ok := c.Config(); ok {
		t.Fatal("Lock 后不应可发信")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
