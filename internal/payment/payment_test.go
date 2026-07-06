// 支付测试 / Payment Tests
// 功能：Stripe 验签（合法/篡改/过期/格式）、Webhook 双校验+幂等（同一事务）、密钥缓存生命周期
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

const testWhsec = "whsec_test_secret_0123456789"

// signStripe 按 Stripe 方案对原始字节签名，构造 Stripe-Signature 头。
func signStripe(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func eventBody(id, ref, payStatus string, amount int64, currency string) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"type":"checkout.session.completed","data":{"object":{"client_reference_id":%q,"payment_status":%q,"amount_total":%d,"currency":%q}}}`,
		id, ref, payStatus, amount, currency))
}

// ---- 验签纯函数 ----

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	now := time.Unix(1_700_000_000, 0)
	hdr := signStripe(testWhsec, now.Unix(), body)

	// 合法。
	if err := verifyStripeSignature(body, hdr, testWhsec, defaultTolerance, now); err != nil {
		t.Fatalf("合法签名应通过: %v", err)
	}
	// 篡改负载 → 不匹配。
	if err := verifyStripeSignature([]byte(`{"hello":"evil"}`), hdr, testWhsec, defaultTolerance, now); err != ErrSigMismatch {
		t.Fatalf("篡改负载应 ErrSigMismatch: %v", err)
	}
	// 错误密钥 → 不匹配。
	if err := verifyStripeSignature(body, hdr, "whsec_wrong", defaultTolerance, now); err != ErrSigMismatch {
		t.Fatalf("错误密钥应 ErrSigMismatch: %v", err)
	}
	// 过期（超出容差）→ 防重放。
	later := now.Add(defaultTolerance + time.Minute)
	if err := verifyStripeSignature(body, hdr, testWhsec, defaultTolerance, later); err != ErrSigExpired {
		t.Fatalf("超容差应 ErrSigExpired: %v", err)
	}
	// 格式非法。
	if err := verifyStripeSignature(body, "garbage", testWhsec, defaultTolerance, now); err != ErrSigFormat {
		t.Fatalf("非法头应 ErrSigFormat: %v", err)
	}
}

// ---- Webhook 双校验 + 幂等（带库）----

func newTestSvc(t *testing.T) (*Service, *sql.DB, []byte) {
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
	settingsSvc := settings.New(db)
	cache := NewKeyCache(settingsSvc)
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	ctx := context.Background()
	if err := settingsSvc.SetEncrypted(ctx, KeyStripeWebhookSecret, []byte(testWhsec), kek); err != nil {
		t.Fatal(err)
	}
	if err := settingsSvc.SetEncrypted(ctx, KeyStripeSecret, []byte("sk_test_x"), kek); err != nil {
		t.Fatal(err)
	}
	if err := cache.Unlock(ctx, kek); err != nil {
		t.Fatal(err)
	}
	return NewService(db, settingsSvc, cache), db, kek
}

// seedOrder 直插一张 pending 订单，返回其 public_id。
func seedOrder(t *testing.T, db *sql.DB, ref string, totalCents int64, currency string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO customer (public_id, email, name) VALUES ('cust-1','a@b.com','A')`); err != nil {
		t.Fatal(err)
	}
	var custID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM customer WHERE public_id='cust-1'`).Scan(&custID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO "order" (public_id, customer_id, status, email, ship_name, ship_address, currency, subtotal_cents, total_cents)
		 VALUES (?, ?, 'pending', 'a@b.com', 'A', 'addr', ?, ?, ?)`,
		ref, custID, currency, totalCents, totalCents); err != nil {
		t.Fatal(err)
	}
}

func orderStatus(t *testing.T, db *sql.DB, ref string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT status FROM "order" WHERE public_id=?`, ref).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func eventCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM webhook_event`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWebhookHappyPathAndIdempotent(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	ref := "order-uuid-1"
	seedOrder(t, db, ref, 2500, "USD")

	body := eventBody("evt_1", ref, "paid", 2500, "usd")
	hdr := signStripe(testWhsec, time.Now().Unix(), body)

	// 首投：订单转 paid。
	if err := svc.HandleWebhook(ctx, body, hdr); err != nil {
		t.Fatalf("首投应成功: %v", err)
	}
	if st := orderStatus(t, db, ref); st != "paid" {
		t.Fatalf("订单应 paid，得 %s", st)
	}
	if n := eventCount(t, db); n != 1 {
		t.Fatalf("应有 1 条事件，得 %d", n)
	}

	// 重投同一事件（Stripe 会重发）：幂等，订单仍 paid，事件不重复。
	if err := svc.HandleWebhook(ctx, body, hdr); err != nil {
		t.Fatalf("重投应幂等成功: %v", err)
	}
	if n := eventCount(t, db); n != 1 {
		t.Fatalf("重投后仍应 1 条事件，得 %d", n)
	}
	if st := orderStatus(t, db, ref); st != "paid" {
		t.Fatalf("重投后订单仍应 paid，得 %s", st)
	}
}

func TestWebhookForgedSignatureRejected(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	ref := "order-uuid-2"
	seedOrder(t, db, ref, 2500, "USD")

	body := eventBody("evt_2", ref, "paid", 2500, "usd")
	// 用错误密钥签名 = 伪造。
	hdr := signStripe("whsec_attacker", time.Now().Unix(), body)

	if err := svc.HandleWebhook(ctx, body, hdr); err == nil {
		t.Fatal("伪造签名应被拒")
	}
	if st := orderStatus(t, db, ref); st != "pending" {
		t.Fatalf("伪造未改单，订单应仍 pending，得 %s", st)
	}
	if n := eventCount(t, db); n != 0 {
		t.Fatalf("伪造不应记录事件，得 %d", n)
	}
}

func TestWebhookAmountMismatchRejected(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	ref := "order-uuid-3"
	seedOrder(t, db, ref, 2500, "USD") // 库内 2500

	// 签名真，但金额被改成 1（张冠李戴/篡改）。
	body := eventBody("evt_3", ref, "paid", 1, "usd")
	hdr := signStripe(testWhsec, time.Now().Unix(), body)

	if err := svc.HandleWebhook(ctx, body, hdr); err != ErrMismatch {
		t.Fatalf("金额不符应 ErrMismatch: %v", err)
	}
	if st := orderStatus(t, db, ref); st != "pending" {
		t.Fatalf("不符未改单，订单应仍 pending，得 %s", st)
	}
	if n := eventCount(t, db); n != 0 {
		t.Fatalf("不符应回滚事件记录，得 %d", n)
	}
}

func TestWebhookUnpaidIgnored(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	ref := "order-uuid-4"
	seedOrder(t, db, ref, 2500, "USD")

	// 事件类型对，但 payment_status 非 paid → 不改单（确认收到）。
	body := eventBody("evt_4", ref, "unpaid", 2500, "usd")
	hdr := signStripe(testWhsec, time.Now().Unix(), body)

	if err := svc.HandleWebhook(ctx, body, hdr); err != nil {
		t.Fatalf("未付事件应安全忽略: %v", err)
	}
	if st := orderStatus(t, db, ref); st != "pending" {
		t.Fatalf("未付不应改单，订单应仍 pending，得 %s", st)
	}
}

func TestWebhookLockedReturnsErrLocked(t *testing.T) {
	// 不解锁缓存 → 锁定。
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	settingsSvc := settings.New(db)
	cache := NewKeyCache(settingsSvc) // 从不 Unlock
	svc := NewService(db, settingsSvc, cache)

	body := eventBody("evt_5", "order-x", "paid", 2500, "usd")
	hdr := signStripe(testWhsec, time.Now().Unix(), body)
	if err := svc.HandleWebhook(context.Background(), body, hdr); err != ErrLocked {
		t.Fatalf("锁定应返回 ErrLocked（让网关重投）: %v", err)
	}
}

// ---- 退款（M3.3a）----

func seedPaidOrder(t *testing.T, db *sql.DB, ref string, cents int64, payRef string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO customer (public_id, email, name) VALUES (?, ?, 'A')`, "c-"+ref, ref+"@b.com"); err != nil {
		t.Fatal(err)
	}
	var cid int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM customer WHERE public_id=?`, "c-"+ref).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO "order" (public_id, customer_id, status, email, ship_name, ship_address, currency, subtotal_cents, total_cents, payment_provider, payment_ref)
		 VALUES (?, ?, 'paid', ?, 'A', 'addr', 'USD', ?, ?, 'stripe', ?)`,
		ref, cid, ref+"@b.com", cents, cents, payRef); err != nil {
		t.Fatal(err)
	}
}

func refundCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM refund`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func chargeRefundedBody(id, paymentIntent string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"type":"charge.refunded","data":{"object":{"payment_intent":%q,"currency":"usd"}}}`, id, paymentIntent))
}

func TestRefundHappyPathAndBlocksDouble(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/refunds" {
			_, _ = w.Write([]byte(`{"id":"re_test_1"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL

	seedPaidOrder(t, db, "ORD-R", 2500, "pi_123")
	if err := svc.Refund(ctx, "ORD-R"); err != nil {
		t.Fatalf("退款应成功: %v", err)
	}
	if st := orderStatus(t, db, "ORD-R"); st != "refunded" {
		t.Fatalf("订单应 refunded，得 %s", st)
	}
	if n := refundCount(t, db); n != 1 {
		t.Fatalf("应有 1 条退款记录，得 %d", n)
	}
	// 已退款再退 → 被拒（状态不可退）。
	if err := svc.Refund(ctx, "ORD-R"); err != ErrNotRefundable {
		t.Fatalf("重复退款应 ErrNotRefundable: %v", err)
	}
}

func TestRefundNotRefundableWhenPending(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	seedOrder(t, db, "ORD-P", 2500, "USD") // pending
	if err := svc.Refund(ctx, "ORD-P"); err != ErrNotRefundable {
		t.Fatalf("未付订单退款应 ErrNotRefundable: %v", err)
	}
	if st := orderStatus(t, db, "ORD-P"); st != "pending" {
		t.Fatalf("订单应仍 pending，得 %s", st)
	}
}

func TestChargeRefundedWebhookSyncsStatus(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	ctx := context.Background()
	seedPaidOrder(t, db, "ORD-CW", 2500, "pi_w")

	body := chargeRefundedBody("evt_cr1", "pi_w")
	hdr := signStripe(testWhsec, time.Now().Unix(), body)

	if err := svc.HandleWebhook(ctx, body, hdr); err != nil {
		t.Fatalf("charge.refunded 应处理: %v", err)
	}
	if st := orderStatus(t, db, "ORD-CW"); st != "refunded" {
		t.Fatalf("订单应同步 refunded，得 %s", st)
	}
	// 幂等重投。
	if err := svc.HandleWebhook(ctx, body, hdr); err != nil {
		t.Fatalf("重投应幂等: %v", err)
	}
	if st := orderStatus(t, db, "ORD-CW"); st != "refunded" {
		t.Fatalf("重投后仍 refunded，得 %s", st)
	}
	if n := eventCount(t, db); n != 1 {
		t.Fatalf("应只 1 条事件，得 %d", n)
	}
}

// ---- 环境变量覆盖旁路（env > 加密库，覆盖非双写）----

func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveEnvKeys(t *testing.T) {
	// 未设 secret → 不激活。
	if e := resolveStripeEnv(fakeEnv(map[string]string{})); e.active {
		t.Fatal("未设 STRIPE_SECRET_KEY 不应激活")
	}
	// 设了 secret → 激活，mode 由前缀推断。
	e := resolveStripeEnv(fakeEnv(map[string]string{
		envStripeSecret:  "sk_live_abc",
		envStripeWebhook: "whsec_env",
	}))
	if !e.active || e.mode != "live" || e.webhook != "whsec_env" {
		t.Fatalf("激活/推断有误: %+v", e)
	}
	// 显式 mode 覆盖推断。
	e2 := resolveStripeEnv(fakeEnv(map[string]string{envStripeSecret: "sk_live_x", envStripeMode: "test"}))
	if e2.mode != "test" {
		t.Fatalf("显式 mode 应覆盖推断，得 %s", e2.mode)
	}
	// 受限密钥(rk_，Stripe 推荐)live 也要判对，不被误判 test。
	if e3 := resolveStripeEnv(fakeEnv(map[string]string{envStripeSecret: "rk_live_y"})); e3.mode != "live" {
		t.Fatalf("rk_live_ 应判 live，得 %s", e3.mode)
	}
	if e4 := resolveStripeEnv(fakeEnv(map[string]string{envStripeSecret: "rk_test_z"})); e4.mode != "test" {
		t.Fatalf("rk_test_ 应判 test，得 %s", e4.mode)
	}
}

func TestKeyCacheEnvOverridesDB(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	settingsSvc := settings.New(db)
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	// 库里存了一套 key（应被 env 覆盖、绝不被读取）。
	_ = settingsSvc.SetEncrypted(ctx, KeyStripeSecret, []byte("sk_db_should_not_win"), kek)
	_ = settingsSvc.SetEncrypted(ctx, KeyStripeWebhookSecret, []byte("whsec_db"), kek)

	cache := &KeyCache{settings: settingsSvc, stripeEnv: resolveStripeEnv(fakeEnv(map[string]string{
		envStripeSecret:      "sk_env_wins",
		envStripeWebhook:     "whsec_env_wins",
		envStripePublishable: "pk_env",
	}))}

	// Unlock 在 env 模式下是 no-op：不读/不解密库内值。
	if err := cache.Unlock(ctx, kek); err != nil {
		t.Fatal(err)
	}
	if v, ok := cache.secretKey(); !ok || v != "sk_env_wins" {
		t.Fatalf("env 应覆盖库，得 %q", v)
	}
	if v, ok := cache.webhookSecret(); !ok || v != "whsec_env_wins" {
		t.Fatalf("env whsec 应生效，得 %q", v)
	}
	st := cache.Status()
	if st.StripeSource != "env" || !st.StripeHasSecret || st.StripePublishable != "pk_env" {
		t.Fatalf("Status 应报 env 来源: %+v", st)
	}
	// Lock 不影响 env。
	cache.Lock()
	if v, _ := cache.secretKey(); v != "sk_env_wins" {
		t.Fatal("Lock 不应清掉 env 覆盖")
	}
}

func TestWebhookEnvOverrideNoLoginNeeded(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	settingsSvc := settings.New(db)
	// env 覆盖：whsec 来自环境变量；从不登录、从不 Unlock。
	cache := &KeyCache{settings: settingsSvc, stripeEnv: resolveStripeEnv(fakeEnv(map[string]string{
		envStripeSecret:  "sk_test_env",
		envStripeWebhook: testWhsec,
	}))}
	svc := NewService(db, settingsSvc, cache)

	ctx := context.Background()
	ref := "order-env-1"
	seedOrder(t, db, ref, 2500, "USD")
	body := eventBody("evt_env_1", ref, "paid", 2500, "usd")
	hdr := signStripe(testWhsec, time.Now().Unix(), body)

	// env 模式下「锁定→503」不触发：直接验签通过并改单。
	if err := svc.HandleWebhook(ctx, body, hdr); err != nil {
		t.Fatalf("env 模式无需登录即应处理: %v", err)
	}
	if st := orderStatus(t, db, ref); st != "paid" {
		t.Fatalf("订单应 paid，得 %s", st)
	}
}

func TestEnvHalfSetNoFallbackToDB(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	settingsSvc := settings.New(db)
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	// 库里有 whsec —— 半设 env 模式下绝不能被取用。
	_ = settingsSvc.SetEncrypted(ctx, KeyStripeWebhookSecret, []byte("whsec_db_must_not_leak"), kek)

	// 仅设 env secret，未设 env whsec。
	cache := &KeyCache{settings: settingsSvc, stripeEnv: resolveStripeEnv(fakeEnv(map[string]string{
		envStripeSecret: "sk_test_only",
	}))}
	_ = cache.Unlock(ctx, kek) // env 模式 no-op

	if _, ok := cache.secretKey(); !ok {
		t.Fatal("env secret 应可用")
	}
	// 关键：whsec 必须 false（拒绝半回退到库），而非取到库内值。
	if v, ok := cache.webhookSecret(); ok {
		t.Fatalf("半设不得回退取库内 whsec，得 %q", v)
	}
	// Status 反映不完整。
	if st := cache.Status(); st.StripeSource != "env" || st.StripeHasWebhook {
		t.Fatalf("半设 Status 应 env+HasWebhook=false: %+v", st)
	}
}

// ---- 密钥缓存生命周期 ----

func TestKeyCacheLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	settingsSvc := settings.New(db)
	cache := NewKeyCache(settingsSvc)
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)

	// 初始锁定。
	if _, ok := cache.webhookSecret(); ok {
		t.Fatal("初始应锁定")
	}
	// 存密钥并解锁。
	_ = settingsSvc.SetEncrypted(ctx, KeyStripeSecret, []byte("sk_test_a"), kek)
	_ = settingsSvc.SetEncrypted(ctx, KeyStripeWebhookSecret, []byte(testWhsec), kek)
	if err := cache.Unlock(ctx, kek); err != nil {
		t.Fatal(err)
	}
	if v, ok := cache.webhookSecret(); !ok || v != testWhsec {
		t.Fatalf("解锁后应取到 whsec，得 %q ok=%v", v, ok)
	}
	if v, ok := cache.secretKey(); !ok || v != "sk_test_a" {
		t.Fatalf("解锁后应取到 sk，得 %q ok=%v", v, ok)
	}
	// 退出销毁。
	cache.Lock()
	if _, ok := cache.secretKey(); ok {
		t.Fatal("Lock 后应销毁")
	}
}

// TestStripeVersionHeaderPinned 断言所有出站 Stripe 请求都带 Stripe-Version 头，
// 且值 == stripeAPIVersion 常量（债2：钉死 API 版本，防账号默认版本被平台侧推进致静默字段错位）。
func TestStripeVersionHeaderPinned(t *testing.T) {
	svc, _, _ := newTestSvc(t)
	ctx := context.Background()

	seen := map[string]string{} // path -> Stripe-Version 头
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Stripe-Version")
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			_, _ = w.Write([]byte(`{"id":"cs_test_1","url":"https://checkout.stripe.test/x"}`))
		case "/v1/refunds":
			_, _ = w.Write([]byte(`{"id":"re_test_1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL

	// 出站请求一：建 Checkout 会话。
	if _, err := svc.stripe.CreatePayment(ctx, OrderForPayment{
		PublicID: "ORD-V", Currency: "USD", AmountCents: 2500,
		SuccessURL: "https://x/s", CancelURL: "https://x/c",
	}); err != nil {
		t.Fatalf("CreatePayment 应成功: %v", err)
	}
	// 出站请求二：退款（直接调 provider，绕过订单状态编排）。
	if _, err := svc.stripe.Refund(ctx, "pi_test", 2500); err != nil {
		t.Fatalf("Refund 应成功: %v", err)
	}

	if stripeAPIVersion == "" {
		t.Fatal("stripeAPIVersion 常量不应为空")
	}
	for _, path := range []string{"/v1/checkout/sessions", "/v1/refunds"} {
		got, ok := seen[path]
		if !ok {
			t.Fatalf("未捕获到 %s 的出站请求", path)
		}
		if got != stripeAPIVersion {
			t.Fatalf("%s 的 Stripe-Version=%q，期望钉死常量 %q", path, got, stripeAPIVersion)
		}
	}
}
