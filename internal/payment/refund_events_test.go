// 退款通知恢复测试 / Refund Event Recovery Tests
// 功能：覆盖签名校验、丢失编号恢复、乱序通知及成功后失败的持久核对边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 23:55:00
package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/order"
)

func refundStatusEvent(id, kind, requestID, ref string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"type":%q,"data":{"object":{"id":"re-1","payment_intent":%q,"amount":500,"currency":"usd","metadata":{"kartwo_refund_id":%q}}}}`, id, kind, ref, requestID))
}

func TestRefundLookupRejectsURLAndPathFragments(t *testing.T) {
	svc, _, _ := newTestSvc(t)
	for _, ref := range []string{"", "https://localhost/refund", "../refund", "re/../../x", "re?secret=x", "re%2fother", "re\nheader", strings.Repeat("a", 256)} {
		for _, provider := range []string{"stripe", "paypal"} {
			if _, err := svc.refundGateway(provider).lookupRefund(context.Background(), ref); err != ErrMismatch {
				t.Fatalf("%s accepted non-identifier %q: %v", provider, ref, err)
			}
		}
	}
}

func TestRefundEventRecoversLostResultAndIgnoresStaleStatus(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "event-recovery", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.loseFirst = true
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "event-recovery", refundInput("event-recovery-001", 500, 0))
	if err != nil || r.State != "pending" {
		t.Fatalf("lost response: %+v %v", r, err)
	}
	if _, err := db.Exec(`UPDATE refund_request SET first_sent_at=?,state='review',reason='unknown_result'`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	// Even a stale notification is safe: retrieve the current resource, never POST.
	body := refundStatusEvent("refund-notice", "refund.created", "", "pi-refund")
	if err := svc.HandleWebhook(context.Background(), body, "invalid"); err == nil {
		t.Fatal("unsigned notification accepted")
	}
	f.mu.Lock()
	lookups := f.lookups
	f.mu.Unlock()
	if lookups != 0 {
		t.Fatal("unverified event reached provider")
	}
	for range 2 {
		if err := deliverStripe(t, svc, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := deliverStripe(t, svc, refundStatusEvent("old-pending-notice", "refund.updated", r.ID, "pi-refund")); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 2500, 500, 1)
	var state, gateway string
	if err := db.QueryRow(`SELECT state,gateway_ref FROM refund_request`).Scan(&state, &gateway); err != nil || state != "succeeded" || gateway != "re-1" {
		t.Fatalf("not recovered: %s %s %v", state, gateway, err)
	}
	if posts, _ := f.counts(); posts != 1 || refundCount(t, db) != 1 {
		t.Fatal("event recovery duplicated a refund")
	}
}

func TestRefundEventFailureAfterSuccessRequiresReconciliation(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "later-failed", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "later-failed", refundInput("later-failed-001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	f.state(r.ID, "failed")
	for _, eventID := range []string{"failure-notice", "failure-notice", "failure-notice-2"} {
		if err := deliverStripe(t, svc, refundStatusEvent(eventID, "refund.failed", r.ID, "pi-refund")); err != nil {
			t.Fatal(err)
		}
	}
	assertRefundUnderReview(t, svc, db, "later-failed")
	if _, err := svc.RequestRefund(context.Background(), "later-failed", refundInput("later-failed-002", 500, 500)); err != ErrRefundConflict {
		t.Fatalf("new refund allowed before reconciliation: %v", err)
	}
	result, err := svc.RetryRefund(context.Background(), "later-failed", r.ID, "")
	if err != nil || result.State != "review" {
		t.Fatalf("cached success still advertised: %+v %v", result, err)
	}
	// An older aggregate callback must not silently clear the review barrier.
	if err := deliverStripe(t, svc, stripeRefund("old-aggregate", "pi-refund", 2500, 500, "usd")); err != nil {
		t.Fatal(err)
	}
	assertRefundUnderReview(t, svc, db, "later-failed")
	if posts, _ := f.counts(); posts != 1 {
		t.Fatal("failure notification caused another refund")
	}
}

func assertRefundUnderReview(t *testing.T, svc *Service, db *sql.DB, id string) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM refund_reconciliation WHERE state='open'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("persistent review issue: %d %v", n, err)
	}
	os := order.New(db, svc.settings)
	o, err := os.AdminGet(context.Background(), id)
	if err != nil || o.Status != "payment_review" || !o.PaymentReview || o.PaymentStatus != "review" || !o.Receipts[0].RefundReconciliationRequired || o.RefundRequests[0].State != "reconciliation" {
		t.Fatalf("financial view did not flag uncertainty: %+v %v", o, err)
	}
	if err := os.Fulfill(context.Background(), id, "carrier", "tracking", ""); err != sql.ErrNoRows {
		t.Fatalf("fulfillment allowed: %v", err)
	}
	stats, err := os.DashboardStats(context.Background())
	if err != nil || stats.TodaySalesCents != 0 {
		t.Fatalf("uncertain payment counted as net sales: %+v %v", stats, err)
	}
	// Preserve historical postings until a complete reconciliation, rather than
	// guessing a delta from possibly overlapping cumulative callback amounts.
	assertLedger(t, db, 2500, 500, 1)
}

func TestRefundEventMismatchDoesNotConsumeEvent(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "event-mismatch", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.initialState = "pending"
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "event-mismatch", refundInput("event-mismatch-001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	body := refundStatusEvent("mismatch-notice", "refund.updated", r.ID, "wrong-payment")
	if err := deliverStripe(t, svc, body); err != ErrMismatch {
		t.Fatalf("mismatched payment accepted: %v", err)
	}
	if eventCount(t, db) != 0 {
		t.Fatal("mismatch consumed event")
	}
	f.state(r.ID, "failed")
	if err := deliverStripe(t, svc, refundStatusEvent("mismatch-notice", "refund.failed", r.ID, "pi-refund")); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM refund_request`).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("pending failure not synchronized: %s %v", state, err)
	}
	assertLedger(t, db, 2500, 0, 1)
}

func TestPayPalRefundEventFailureAfterSuccess(t *testing.T) {
	var requestID string
	var currentState atomic.Value
	currentState.Store("COMPLETED")
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
		case r.URL.Path == "/v1/notifications/verify-webhook-signature":
			_, _ = w.Write([]byte(`{"verification_status":"SUCCESS"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/refund"):
			requestID = r.Header.Get("PayPal-Request-Id")
			w.WriteHeader(503) // The platform accepted the refund, but its response was lost.
		case r.Method == http.MethodGet && r.URL.Path == "/v2/payments/refunds/PP-refund":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "PP-refund", "status": currentState.Load(), "custom_id": requestID, "amount": paypalMoney{Value: "5.00", Currency: "USD"}, "seller_payable_breakdown": map[string]any{"total_refunded_amount": paypalMoney{Value: "5.00", Currency: "USD"}}, "links": []map[string]string{{"href": "https://api.paypal.test/v2/payments/captures/cap-refund", "rel": "up"}}})
		default:
			w.WriteHeader(404)
		}
	}
	svc, db, _ := newPayPalSvc(t, handler)
	svc.keys.paypalEnv.webhookID = "webhook-id"
	seedPaidOrder(t, db, "pp-failed", 2500, "cap-refund")
	if _, err := db.Exec(`UPDATE "order" SET payment_provider='paypal'`); err != nil {
		t.Fatal(err)
	}
	r, err := svc.RequestRefund(context.Background(), "pp-failed", RefundInput{ClientKey: "paypal-failed-001", Provider: "paypal", PaymentRef: "cap-refund", AmountCents: 500})
	if err != nil || r.State != "pending" {
		t.Fatalf("initial refund: %+v %v", r, err)
	}
	body := []byte(fmt.Sprintf(`{"id":"paypal-failed-notice","event_type":"PAYMENT.REFUND.FAILED","resource":{"id":"PP-refund","custom_id":%q,"amount":{"value":"5.00","currency_code":"USD"},"links":[{"href":"https://api.paypal.test/v2/payments/captures/cap-refund","rel":"up"}]}}`, r.ID))
	headers := http.Header{"Paypal-Transmission-Id": {"transmission"}, "Paypal-Transmission-Time": {"now"}, "Paypal-Cert-Url": {"https://api.paypal.com/cert"}, "Paypal-Auth-Algo": {"SHA256withRSA"}, "Paypal-Transmission-Sig": {"signature"}}
	completed := []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "paypal-failed-notice", "paypal-completed-notice"), "PAYMENT.REFUND.FAILED", "PAYMENT.CAPTURE.REFUNDED"))
	if err := svc.HandlePayPalWebhook(context.Background(), completed, headers); err != nil {
		t.Fatalf("completion did not recover unknown result: %v", err)
	}
	assertLedger(t, db, 2500, 500, 1)
	currentState.Store("FAILED")
	if err := svc.HandlePayPalWebhook(context.Background(), body, headers); err != nil {
		t.Fatal(err)
	}
	assertRefundUnderReview(t, svc, db, "pp-failed")
}
