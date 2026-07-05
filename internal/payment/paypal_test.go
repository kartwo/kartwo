// PayPal 测试 / PayPal Tests
// 功能：金额分↔小数串、AvailableMethods、建单(审批URL)、同步 capture→paid、金额不符拒
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-23 00:20:52
package payment

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func TestCentsDecimalRoundtrip(t *testing.T) {
	cases := []struct {
		cents int64
		str   string
	}{{9900, "99.00"}, {5, "0.05"}, {100, "1.00"}, {0, "0.00"}, {123456, "1234.56"}}
	for _, c := range cases {
		if got := centsToDecimal(c.cents); got != c.str {
			t.Fatalf("centsToDecimal(%d)=%q 期望 %q", c.cents, got, c.str)
		}
		back, err := decimalToCents(c.str)
		if err != nil || back != c.cents {
			t.Fatalf("decimalToCents(%q)=%d,%v 期望 %d", c.str, back, err, c.cents)
		}
	}
	// 单位小数补齐。
	if v, _ := decimalToCents("12.3"); v != 1230 {
		t.Fatalf("decimalToCents(12.3)=%d 期望 1230", v)
	}
}

func newPayPalSvc(t *testing.T, handler http.HandlerFunc) (*Service, *sql.DB, *httptest.Server) {
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
	// PayPal 由 env 覆盖提供凭证（无需登录解锁）。
	cache := &KeyCache{settings: settingsSvc, paypalEnv: resolvePayPalEnv(fakeEnv(map[string]string{
		envPayPalClientID: "id_x", envPayPalSecret: "sec_x",
	}))}
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	svc := NewService(db, settingsSvc, cache)
	svc.paypal.apiBase = ts.URL
	return svc, db, ts
}

func paypalMock(captureBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case r.URL.Path == "/v2/checkout/orders":
			_, _ = w.Write([]byte(`{"id":"PPORDER1","links":[{"rel":"payer-action","href":"https://paypal.test/approve?token=PPORDER1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/capture"):
			_, _ = w.Write([]byte(captureBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestAvailableMethodsPayPalOnly(t *testing.T) {
	svc, _, _ := newPayPalSvc(t, paypalMock(""))
	// 美国市场含 stripe+paypal；仅 paypal 配了凭证 → 只返 paypal。
	ms := svc.AvailableMethods(context.Background())
	if len(ms) != 1 || ms[0] != "paypal" {
		t.Fatalf("AvailableMethods 应只 paypal，得 %v", ms)
	}
}

func TestPayPalStartCheckout(t *testing.T) {
	svc, _, _ := newPayPalSvc(t, paypalMock(""))
	url, err := svc.StartCheckout(context.Background(), "paypal", OrderForPayment{
		PublicID: "ORD-PP", Currency: "USD", AmountCents: 9900,
		SuccessURL: "http://shop/paypal/return", CancelURL: "http://shop/order/ORD-PP",
	})
	if err != nil {
		t.Fatalf("建单应成功: %v", err)
	}
	if !strings.Contains(url, "/approve") {
		t.Fatalf("应返回审批 URL，得 %q", url)
	}
}

func TestCapturePayPalMarksPaid(t *testing.T) {
	// 还原 PayPal capture 响应真实结构：custom_id 在 capture 对象上（而非 purchase_unit 顶层）。
	// 回归护栏——曾因读错层级导致 custom_id 恒空、钱已扣却订单不转 paid（见 DECISIONS 2026-06-25）。
	body := `{"status":"COMPLETED","purchase_units":[{"payments":{"captures":[{"id":"CAP1","custom_id":"ORD-PP","amount":{"currency_code":"USD","value":"99.00"}}]}}]}`
	svc, db, _ := newPayPalSvc(t, paypalMock(body))
	ctx := context.Background()
	seedOrder(t, db, "ORD-PP", 9900, "USD")

	ref, err := svc.CapturePayPal(ctx, "PPORDER1")
	if err != nil {
		t.Fatalf("capture 应成功: %v", err)
	}
	if ref != "ORD-PP" {
		t.Fatalf("订单 ref=%q", ref)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "paid" {
		t.Fatalf("订单应 paid，得 %s", st)
	}
	// 落了 payment_ref=capture id、provider=paypal。
	var prov, payref string
	_ = db.QueryRow(`SELECT payment_provider, payment_ref FROM "order" WHERE public_id='ORD-PP'`).Scan(&prov, &payref)
	if prov != "paypal" || payref != "CAP1" {
		t.Fatalf("支付引用落库不符: provider=%s ref=%s", prov, payref)
	}
}

func TestCapturePayPalCustomIDAtPurchaseUnit(t *testing.T) {
	// 兼容性护栏：部分流程把 custom_id 放在 purchase_unit 顶层——顶层在时优先取，回退不应吞掉它。
	body := `{"status":"COMPLETED","purchase_units":[{"custom_id":"ORD-PP","payments":{"captures":[{"id":"CAP1","amount":{"currency_code":"USD","value":"99.00"}}]}}]}`
	svc, db, _ := newPayPalSvc(t, paypalMock(body))
	ctx := context.Background()
	seedOrder(t, db, "ORD-PP", 9900, "USD")

	ref, err := svc.CapturePayPal(ctx, "PPORDER1")
	if err != nil {
		t.Fatalf("capture 应成功: %v", err)
	}
	if ref != "ORD-PP" {
		t.Fatalf("订单 ref=%q", ref)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "paid" {
		t.Fatalf("订单应 paid，得 %s", st)
	}
}

func seedPaidPayPal(t *testing.T, db *sql.DB, ref string, cents int64, captureID string) {
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
		 VALUES (?, ?, 'paid', ?, 'A', 'addr', 'USD', ?, ?, 'paypal', ?)`,
		ref, cid, ref+"@b.com", cents, cents, captureID); err != nil {
		t.Fatal(err)
	}
}

func TestPayPalRefund(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, "/refund"):
			_, _ = w.Write([]byte(`{"id":"RF1","status":"COMPLETED"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
	svc, db, _ := newPayPalSvc(t, handler)
	ctx := context.Background()
	seedPaidPayPal(t, db, "ORD-PP", 9900, "CAP1")

	if err := svc.Refund(ctx, "ORD-PP"); err != nil {
		t.Fatalf("PayPal 退款应成功: %v", err)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "refunded" {
		t.Fatalf("订单应 refunded，得 %s", st)
	}
	var prov, rid string
	_ = db.QueryRow(`SELECT provider, provider_refund_id FROM refund WHERE provider_refund_id='RF1'`).Scan(&prov, &rid)
	if prov != "paypal" || rid != "RF1" {
		t.Fatalf("退款记录不符: provider=%s id=%s", prov, rid)
	}
}

// paypalWebhookSvc 构造带 webhook_id 的服务 + 可控验签结果的 mock。
func paypalWebhookSvc(t *testing.T, verifyStatus string) (*Service, *sql.DB) {
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
	cache := &KeyCache{settings: settingsSvc, paypalEnv: resolvePayPalEnv(fakeEnv(map[string]string{
		envPayPalClientID: "id_x", envPayPalSecret: "sec_x", envPayPalWebhookID: "WH-1",
	}))}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/v1/notifications/verify-webhook-signature":
			_, _ = w.Write([]byte(`{"verification_status":"` + verifyStatus + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	svc := NewService(db, settingsSvc, cache)
	svc.paypal.apiBase = ts.URL
	return svc, db
}

func TestPayPalWebhookCaptureCompleted(t *testing.T) {
	svc, db := paypalWebhookSvc(t, "SUCCESS")
	ctx := context.Background()
	seedOrder(t, db, "ORD-PP", 9900, "USD") // pending
	body := []byte(`{"id":"EVT1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP1","custom_id":"ORD-PP","amount":{"value":"99.00","currency_code":"USD"}}}`)

	if err := svc.HandlePayPalWebhook(ctx, body, http.Header{}); err != nil {
		t.Fatalf("COMPLETED 应处理: %v", err)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "paid" {
		t.Fatalf("订单应 paid，得 %s", st)
	}
	// 幂等。
	if err := svc.HandlePayPalWebhook(ctx, body, http.Header{}); err != nil {
		t.Fatalf("重投应幂等: %v", err)
	}
	if n := eventCount(t, db); n != 1 {
		t.Fatalf("事件应 1 条，得 %d", n)
	}
}

func TestPayPalWebhookRefunded(t *testing.T) {
	svc, db := paypalWebhookSvc(t, "SUCCESS")
	ctx := context.Background()
	seedPaidPayPal(t, db, "ORD-PP", 9900, "CAP1")
	body := []byte(`{"id":"EVT2","event_type":"PAYMENT.CAPTURE.REFUNDED","resource":{"id":"RF9","links":[{"rel":"up","href":"https://api/v2/payments/captures/CAP1"}]}}`)

	if err := svc.HandlePayPalWebhook(ctx, body, http.Header{}); err != nil {
		t.Fatalf("REFUNDED 应处理: %v", err)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "refunded" {
		t.Fatalf("订单应 refunded（按 capture id 同步），得 %s", st)
	}
}

func TestPayPalWebhookBadSignature(t *testing.T) {
	svc, db := paypalWebhookSvc(t, "FAILURE")
	ctx := context.Background()
	seedOrder(t, db, "ORD-PP", 9900, "USD")
	body := []byte(`{"id":"EVT3","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP1","custom_id":"ORD-PP","amount":{"value":"99.00","currency_code":"USD"}}}`)

	if err := svc.HandlePayPalWebhook(ctx, body, http.Header{}); !errorIsBadSig(err) {
		t.Fatalf("验签失败应 ErrBadSignature: %v", err)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "pending" {
		t.Fatalf("验签失败不应改单，得 %s", st)
	}
}

func errorIsBadSig(err error) bool { return errors.Is(err, ErrBadSignature) }

func TestCapturePayPalAmountMismatch(t *testing.T) {
	body := `{"status":"COMPLETED","purchase_units":[{"custom_id":"ORD-PP","payments":{"captures":[{"id":"CAP1","amount":{"currency_code":"USD","value":"1.00"}}]}}]}`
	svc, db, _ := newPayPalSvc(t, paypalMock(body))
	ctx := context.Background()
	seedOrder(t, db, "ORD-PP", 9900, "USD") // 库内 9900，capture 报 100

	if _, err := svc.CapturePayPal(ctx, "PPORDER1"); err != ErrMismatch {
		t.Fatalf("金额不符应 ErrMismatch: %v", err)
	}
	if st := orderStatus(t, db, "ORD-PP"); st != "pending" {
		t.Fatalf("不符不应改单，得 %s", st)
	}
}
