// 收款账本回归 / Payment Ledger Regression Tests
// 功能：覆盖异步成功、重复付款、退款乱序与部分退款、迟到回调及历史数据迁移
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:00:00
package payment

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/migrations"
)

func assertLedger(t *testing.T, db *sql.DB, paid, refunded, count int64) {
	t.Helper()
	var gotPaid, gotRefunded, gotCount int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(amount_cents),0),COALESCE(SUM(refunded_cents),0),COUNT(*) FROM payment_receipt`).Scan(&gotPaid, &gotRefunded, &gotCount); err != nil {
		t.Fatal(err)
	}
	if gotPaid != paid || gotRefunded != refunded || gotCount != count {
		t.Fatalf("ledger=(%d,%d,%d), want=(%d,%d,%d)", gotPaid, gotRefunded, gotCount, paid, refunded, count)
	}
}

func deliverStripe(t *testing.T, svc *Service, body []byte) error {
	t.Helper()
	return svc.HandleWebhook(context.Background(), body, signStripe(testWhsec, time.Now().Unix(), body))
}

func stripeRefund(id, ref string, amount, refunded int64, currency string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"type":"charge.refunded","data":{"object":{"payment_intent":%q,"amount":%d,"amount_refunded":%d,"currency":%q}}}`, id, ref, amount, refunded, currency))
}

func TestAsyncPaymentReceiptAndAtomicity(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "async-order", 2500, "USD")
	if err := deliverStripe(t, svc, eventBody("pending-event", "async-order", "unpaid", 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 0, 0, 0)
	body := []byte(strings.ReplaceAll(string(eventBody("success-event", "async-order", "paid", 2500, "usd")), "checkout.session.completed", "checkout.session.async_payment_succeeded"))
	// A failed local transition must not consume the event or leave a receipt.
	if _, err := db.Exec(`CREATE TRIGGER reject_paid BEFORE UPDATE OF status ON "order" WHEN NEW.status='paid' BEGIN SELECT RAISE(ABORT,'test rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := deliverStripe(t, svc, body); err == nil {
		t.Fatal("expected rollback")
	}
	assertLedger(t, db, 0, 0, 0)
	if eventCount(t, db) != 0 {
		t.Fatal("failed event consumed")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_paid`); err != nil {
		t.Fatal(err)
	}
	if err := deliverStripe(t, svc, body); err != nil {
		t.Fatal(err)
	}
	// Both identical event and another successful event for this payment are idempotent.
	if err := deliverStripe(t, svc, body); err != nil {
		t.Fatal(err)
	}
	if err := deliverStripe(t, svc, eventBody("completed-event", "async-order", "paid", 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 2500, 0, 1)
	if orderStatus(t, db, "async-order") != "paid" || eventCount(t, db) != 2 {
		t.Fatal("async success not applied exactly once")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("confirmation count=%d", n)
	}
}

func TestStripePartialRefundOutOfOrderAndFulfilled(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "shipped", 2500, "pi-shipped")
	if _, err := db.Exec(`UPDATE "order" SET status='fulfilled'; INSERT INTO shipment(order_id,carrier,tracking_number) SELECT id,'carrier','track' FROM "order"`); err != nil {
		t.Fatal(err)
	}
	for i, refund := range []int64{500, 500, 200, 2500, 1000} {
		if err := deliverStripe(t, svc, stripeRefund(fmt.Sprintf("refund-%d", i), "pi-shipped", 2500, refund, "usd")); err != nil {
			t.Fatal(err)
		}
		wantStatus, wantRefund := "fulfilled", int64(500)
		if i >= 3 {
			wantStatus, wantRefund = "refunded", 2500
		}
		if got := orderStatus(t, db, "shipped"); got != wantStatus {
			t.Fatalf("step %d status=%s", i, got)
		}
		assertLedger(t, db, 2500, wantRefund, 1)
	}
	var tracking string
	if err := db.QueryRow(`SELECT tracking_number FROM shipment`).Scan(&tracking); err != nil || tracking != "track" {
		t.Fatalf("shipment lost: %s %v", tracking, err)
	}
}

func TestDuplicatePaymentRequiresReviewAndPreservesOriginal(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "duplicate", 2500, "USD")
	if err := deliverStripe(t, svc, eventBody("first", "duplicate", "paid", 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE "order" SET status='fulfilled'; INSERT INTO shipment(order_id,carrier,tracking_number) SELECT id,'carrier','track' FROM "order"`); err != nil {
		t.Fatal(err)
	}
	ev := WebhookEvent{ID: "second", Type: "PAYMENT.CAPTURE.COMPLETED", OrderRef: "duplicate", PaymentRef: "cap-extra", AmountCents: 2500, Currency: "USD"}
	if err := svc.markPaid(context.Background(), "paypal", ev); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 5000, 0, 2)
	if orderStatus(t, db, "duplicate") != "payment_review" {
		t.Fatal("duplicate payment not flagged")
	}
	var ref string
	if err := db.QueryRow(`SELECT payment_ref FROM "order"`).Scan(&ref); err != nil || ref != "pi_duplicate" {
		t.Fatalf("primary reference replaced: %s %v", ref, err)
	}
	// Same capture via synchronous return does not create a third receipt.
	if err := svc.recordPayment(context.Background(), "paypal", ev, false); err != nil {
		t.Fatal(err)
	}
	refund := WebhookEvent{ID: "extra-refund", Type: "PAYMENT.CAPTURE.REFUNDED", PaymentRef: "cap-extra", RefundRef: "rf-extra", AmountCents: 2500, RefundedCents: 2500, Currency: "USD"}
	if err := svc.markRefundedFromWebhook(context.Background(), "paypal", refund); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 5000, 2500, 2)
	if orderStatus(t, db, "duplicate") != "fulfilled" {
		t.Fatal("refund of excess payment must preserve primary paid + shipment")
	}
}

func TestRefundValidationRollbackAndRetryBeforePayment(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "early-refund", 2500, "USD")
	body := stripeRefund("early-refund-event", "pi_early-refund", 2500, 500, "usd")
	if err := deliverStripe(t, svc, body); err != ErrMismatch {
		t.Fatalf("unknown payment: %v", err)
	}
	if eventCount(t, db) != 0 {
		t.Fatal("early refund consumed")
	}
	if err := deliverStripe(t, svc, eventBody("payment", "early-refund", "paid", 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	if err := deliverStripe(t, svc, body); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 2500, 500, 1)
	for _, tc := range []struct {
		amount, refund int64
		currency       string
	}{{2500, 500, "eur"}, {2499, 500, "usd"}, {2500, 2501, "usd"}, {2500, 0, "usd"}} {
		if err := deliverStripe(t, svc, stripeRefund("invalid", "pi_early-refund", tc.amount, tc.refund, tc.currency)); err != ErrMismatch {
			t.Fatalf("invalid refund accepted: %+v %v", tc, err)
		}
	}
	if err := svc.markRefundedFromWebhook(context.Background(), "paypal", WebhookEvent{ID: "wrong-provider", PaymentRef: "pi_early-refund", RefundRef: "r", AmountCents: 500, Currency: "USD"}); err != ErrMismatch {
		t.Fatalf("provider not scoped: %v", err)
	}
	assertLedger(t, db, 2500, 500, 1)
	if eventCount(t, db) != 2 {
		t.Fatal("invalid events consumed")
	}
}

func TestPayPalPartialRefundDedupByRefundID(t *testing.T) {
	svc, db := paypalWebhookSvc(t, "SUCCESS")
	seedPaidPayPal(t, db, "pp", 9900, "cap")
	send := func(event, refund, amount string) error {
		total := amount
		if refund == "rf2" {
			total = "99.00"
			if amount == "80.00" {
				total = "100.00"
			}
		}
		body := []byte(fmt.Sprintf(`{"id":%q,"event_type":"PAYMENT.CAPTURE.REFUNDED","resource":{"id":%q,"seller_payable_breakdown":{"total_refunded_amount":{"value":%q,"currency_code":"USD"}},"amount":{"value":%q,"currency_code":"USD"},"links":[{"rel":"up","href":"https://api.paypal.com/v2/payments/captures/cap"}]}}`, event, refund, total, amount))
		return svc.HandlePayPalWebhook(context.Background(), body, http.Header{})
	}
	if err := send("ev1", "rf1", "20.00"); err != nil {
		t.Fatal(err)
	}
	if err := send("ev2", "rf1", "20.00"); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 9900, 2000, 1)
	if orderStatus(t, db, "pp") != "paid" {
		t.Fatal("partial refund must not close order")
	}
	if err := send("ev-bad", "rf1", "21.00"); err != ErrMismatch {
		t.Fatalf("same refund changed amount: %v", err)
	}
	if err := send("ev-large", "rf2", "80.00"); err != ErrMismatch {
		t.Fatalf("overrefund: %v", err)
	}
	if err := send("ev3", "rf2", "79.00"); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 9900, 9900, 1)
	if orderStatus(t, db, "pp") != "refunded" || refundCount(t, db) != 2 {
		t.Fatal("refund sum incorrect")
	}
}

func TestLegacyReceiptMigrationDoesNotInventFullRefund(t *testing.T) {
	_, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "legacy", 2500, "pi-legacy")
	if _, err := db.Exec(`UPDATE "order" SET status='refunded'; INSERT INTO refund(order_id,provider,provider_refund_id,amount_cents) SELECT id,'stripe','re-legacy',500 FROM "order"; DELETE FROM schema_migrations WHERE version='0029_payment_receipt'; DROP TABLE payment_refund; DROP TABLE payment_receipt`); err != nil {
		t.Fatal(err)
	}
	if n, err := migrate.Run(context.Background(), db, migrations.FS); err != nil || n != 1 {
		t.Fatalf("upgrade: %d %v", n, err)
	}
	assertLedger(t, db, 2500, 500, 1)
	var reason string
	if err := db.QueryRow(`SELECT review_reason FROM payment_receipt`).Scan(&reason); err != nil || reason != "legacy_refund" {
		t.Fatalf("legacy marked as verified full refund: %q %v", reason, err)
	}
	// The migration SQL itself, as well as the runner, is reentrant.
	body, err := fs.ReadFile(migrations.FS, "0029_payment_receipt.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 2500, 500, 1)
	if n, err := migrate.Run(context.Background(), db, migrations.FS); err != nil || n != 0 {
		t.Fatalf("repeat upgrade: %d %v", n, err)
	}
}

func TestReceiptRejectsMissingOrReassignedPaymentReference(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "first", 2500, "USD")
	ev := WebhookEvent{ID: "missing", OrderRef: "first", AmountCents: 2500, Currency: "USD"}
	if err := svc.markPaid(context.Background(), "stripe", ev); err != ErrMismatch {
		t.Fatalf("missing ref: %v", err)
	}
	ev.PaymentRef, ev.ID = "shared-payment", "first-event"
	if err := svc.markPaid(context.Background(), "stripe", ev); err != nil {
		t.Fatal(err)
	}
	seedPaidOrder(t, db, "other", 2500, "pi-other")
	ev.OrderRef, ev.ID = "other", "second-event"
	if err := svc.markPaid(context.Background(), "stripe", ev); err != ErrMismatch {
		t.Fatalf("reassigned ref accepted: %v", err)
	}
	assertLedger(t, db, 2500, 0, 1)
}

func TestRefundUnconfirmedDoesNotChangeFinancialState(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "refund-pending", 2500, "pi-pending")
	var keys []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"re-pending","status":"pending"}`))
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	for range 2 {
		if err := svc.Refund(context.Background(), "refund-pending"); err != ErrRefundUnconfirmed {
			t.Fatalf("pending acknowledged as refunded: %v", err)
		}
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("refund retry key=%v", keys)
	}
	if orderStatus(t, db, "refund-pending") != "paid" || refundCount(t, db) != 0 {
		t.Fatal("pending refund recorded as completed")
	}
}

func TestGatewayDecimalRejectsTruncationAndOverflow(t *testing.T) {
	for _, value := range []string{"1.001", "0.009", "1.", "-0.01", "-1.00", "1e2", "1.0x", "92233720368547758.08", "999999999999999999999999999999.00"} {
		if _, err := decimalToCents(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if got, err := decimalToCents("92233720368547758.07"); err != nil || got != int64(9223372036854775807) {
		t.Fatalf("int64 bound: %d %v", got, err)
	}
}

func TestFinancialViewAndSalesDeductPartialRefund(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "financial", 2500, "USD")
	if err := deliverStripe(t, svc, eventBody("paid", "financial", "paid", 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	if err := deliverStripe(t, svc, stripeRefund("refund", "pi_financial", 2500, 500, "usd")); err != nil {
		t.Fatal(err)
	}
	orders := order.New(db, svc.settings)
	o, err := orders.AdminGet(context.Background(), "financial")
	if err != nil {
		t.Fatal(err)
	}
	if o.PaymentStatus != "partially_refunded" || o.PaidCents != 2500 || o.RefundedCents != 500 || o.FulfillmentStatus != "unfulfilled" || o.PaymentReview {
		t.Fatalf("bad financial view: %+v", o)
	}
	stats, err := orders.DashboardStats(context.Background())
	if err != nil || stats.TodaySalesCents != 2000 {
		t.Fatalf("partial refund sales: %+v %v", stats, err)
	}
	if err := svc.markPaid(context.Background(), "paypal", WebhookEvent{ID: "duplicate", OrderRef: "financial", PaymentRef: "extra", Currency: "USD", AmountCents: 2500}); err != nil {
		t.Fatal(err)
	}
	stats, err = orders.DashboardStats(context.Background())
	if err != nil || stats.TodaySalesCents != 2000 {
		t.Fatalf("duplicate must not inflate sales or remove primary sale: %+v %v", stats, err)
	}
	if err := svc.Refund(context.Background(), "financial"); err != ErrNotRefundable {
		t.Fatalf("must not refund original total after partial refund: %v", err)
	}
}

func TestPayPalPendingCaptureIsNotPaid(t *testing.T) {
	body := `{"status":"COMPLETED","purchase_units":[{"payments":{"captures":[{"id":"CAP1","status":"PENDING","custom_id":"ORD-PP","amount":{"currency_code":"USD","value":"99.00"}}]}}]}`
	svc, db, _ := newPayPalSvc(t, paypalMock(body))
	seedOrder(t, db, "ORD-PP", 9900, "USD")
	if _, err := svc.CapturePayPal(context.Background(), "PPORDER1"); err != ErrPaymentProcessing {
		t.Fatalf("pending capture accepted: %v", err)
	}
	assertLedger(t, db, 0, 0, 0)
	if orderStatus(t, db, "ORD-PP") != "pending" {
		t.Fatal("pending capture marked paid")
	}
}

func TestLateAsyncPaymentAndRefundKeepShipmentBlocked(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "late", 2500, "USD")
	if _, err := db.Exec(`UPDATE "order" SET reservation_expires_at='2020-01-01T00:00:00.000Z'`); err != nil {
		t.Fatal(err)
	}
	body := []byte(strings.ReplaceAll(string(eventBody("late-event", "late", "paid", 2500, "usd")), "checkout.session.completed", "checkout.session.async_payment_succeeded"))
	if err := deliverStripe(t, svc, body); err != nil {
		t.Fatal(err)
	}
	if orderStatus(t, db, "late") != "payment_review" {
		t.Fatal("late payment must not allow fulfillment")
	}
	if err := deliverStripe(t, svc, stripeRefund("partial", "pi_late", 2500, 500, "usd")); err != nil {
		t.Fatal(err)
	}
	if orderStatus(t, db, "late") != "payment_review" {
		t.Fatal("partial refund must not release late-payment hold")
	}
	if err := deliverStripe(t, svc, stripeRefund("full", "pi_late", 2500, 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	if orderStatus(t, db, "late") != "refunded" {
		t.Fatal("late payment refund not resolved")
	}
	var mails int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&mails); err != nil || mails != 0 {
		t.Fatalf("late confirmation=%d %v", mails, err)
	}
}

func TestManualRefundThenWebhookDoesNotDoubleCount(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "manual", 2500, "pi-manual")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		successfulStripeRefund(w, r, "re-manual", "pi-manual", 2500)
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	if _, err := db.Exec(`UPDATE "order" SET status='fulfilled'; INSERT INTO shipment(order_id,carrier,tracking_number) SELECT id,'carrier','track' FROM "order"`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Refund(context.Background(), "manual"); err != nil {
		t.Fatal(err)
	}
	if err := deliverStripe(t, svc, stripeRefund("webhook", "pi-manual", 2500, 2500, "usd")); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 2500, 2500, 1)
	if refundCount(t, db) != 1 || orderStatus(t, db, "manual") != "refunded" {
		t.Fatal("manual + webhook counted twice")
	}
}

func TestConcurrentPaymentNotificationsKeepOneReceipt(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "parallel", 2500, "USD")
	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			independent := NewService(db, svc.settings, svc.keys)
			errs <- independent.markPaid(context.Background(), "stripe", WebhookEvent{ID: fmt.Sprintf("evt-%d", i), OrderRef: "parallel", PaymentRef: "pi-parallel", AmountCents: 2500, Currency: "USD"})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertLedger(t, db, 2500, 0, 1)
	if eventCount(t, db) != workers || orderStatus(t, db, "parallel") != "paid" {
		t.Fatal("concurrent notifications changed financial result")
	}
}

// Full resource fixtures reflect the gateway contract (including refund identity
// and cumulative amount); minimal acknowledgement alone cannot prove success.
func successfulStripeRefund(w http.ResponseWriter, r *http.Request, id, paymentRef string, amount int64) {
	if strings.HasPrefix(r.URL.Path, "/v1/charges/") {
		_, _ = fmt.Fprintf(w, `{"id":"ch-test","payment_intent":%q,"currency":"usd","amount_refunded":%d}`, paymentRef, amount)
	} else {
		_, _ = fmt.Fprintf(w, `{"id":%q,"payment_intent":%q,"currency":"usd","amount":%d,"status":"succeeded","charge":"ch-test","metadata":{"kartwo_refund_id":%q}}`, id, paymentRef, amount, r.Header.Get("Idempotency-Key"))
	}
}
