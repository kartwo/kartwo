// 持久退款测试 / Durable Refund Tests
// 功能：验证指定金额、并发认领、重启恢复、网关结果状态、越权引用和回调乱序
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 23:00:00
package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/settings"
)

type fakeRefundRecord struct {
	ID, Key, State string
	Amount         int64
}
type refundServer struct {
	mu               sync.Mutex
	records          map[string]fakeRefundRecord
	posts, creates   int
	loseFirst        bool
	initialState     string
	entered, release chan struct{}
	callback         func(string, int64)
}

func newRefundServer(t *testing.T) (*refundServer, *httptest.Server) {
	t.Helper()
	f := &refundServer{records: map[string]fakeRefundRecord{}, initialState: "succeeded"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		total := int64(0)
		for _, row := range f.records {
			if row.State == "succeeded" {
				total += row.Amount
			}
		}
		if r.URL.Path == "/v1/charges/ch-refund" {
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"id":"ch-refund","payment_intent":"pi-refund","currency":"usd","amount_refunded":%d}`, total)
			return
		}
		var row fakeRefundRecord
		if r.Method == http.MethodPost && r.URL.Path == "/v1/refunds" {
			if err := r.ParseForm(); err != nil {
				f.mu.Unlock()
				w.WriteHeader(400)
				return
			}
			key := r.Header.Get("Idempotency-Key")
			f.posts++
			row = f.records[key]
			if row.ID == "" {
				amount, err := strconv.ParseInt(r.Form.Get("amount"), 10, 64)
				if err != nil || key == "" || r.Form.Get("metadata[kartwo_refund_id]") != key || r.Form.Get("payment_intent") != "pi-refund" {
					f.mu.Unlock()
					w.WriteHeader(400)
					return
				}
				f.creates++
				row = fakeRefundRecord{ID: fmt.Sprintf("re-%d", f.creates), Key: key, Amount: amount, State: f.initialState}
				f.records[key] = row
			}
			lost := f.loseFirst && f.posts == 1
			entered, release, callback := f.entered, f.release, f.callback
			f.mu.Unlock()
			if entered != nil {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-release
			}
			if callback != nil {
				callback(row.ID, row.Amount)
			}
			if lost {
				w.WriteHeader(503)
				return
			}
		} else if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/refunds/") {
			id := strings.TrimPrefix(r.URL.Path, "/v1/refunds/")
			for _, candidate := range f.records {
				if candidate.ID == id {
					row = candidate
					break
				}
			}
			f.mu.Unlock()
			if row.ID == "" {
				w.WriteHeader(404)
				return
			}
		} else {
			f.mu.Unlock()
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": row.ID, "amount": row.Amount, "currency": "usd", "payment_intent": "pi-refund", "charge": "ch-refund", "status": row.State, "metadata": map[string]string{"kartwo_refund_id": row.Key}})
	}))
	t.Cleanup(ts.Close)
	return f, ts
}
func (f *refundServer) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts, f.creates
}
func (f *refundServer) state(id, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.records[id]
	r.State = state
	f.records[id] = r
}
func refundInput(key string, amount, refunded int64) RefundInput {
	return RefundInput{ClientKey: key, Provider: "stripe", PaymentRef: "pi-refund", AmountCents: amount, ExpectedRefundedCents: refunded}
}

func TestRefundRequestPartialAndReplay(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "refund-order", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	input := refundInput("browser-request-0001", 500, 0)
	first, err := svc.RequestRefund(context.Background(), "refund-order", input)
	if err != nil || first.State != "succeeded" || !first.Created || !first.Completed {
		t.Fatalf("first: %+v %v", first, err)
	}
	assertLedger(t, db, 2500, 500, 1)
	repeat, err := svc.RequestRefund(context.Background(), "refund-order", input)
	if err != nil || repeat.ID != first.ID || repeat.Created || repeat.Completed {
		t.Fatalf("replay: %+v %v", repeat, err)
	}
	changed := input
	changed.AmountCents = 600
	if _, err := svc.RequestRefund(context.Background(), "refund-order", changed); err != ErrRefundConflict {
		t.Fatalf("same key changed amount: %v", err)
	}
	stale := refundInput("browser-request-0002", 500, 0)
	if _, err := svc.RequestRefund(context.Background(), "refund-order", stale); err != ErrRefundConflict {
		t.Fatalf("stale balance accepted: %v", err)
	}
	last, err := svc.RequestRefund(context.Background(), "refund-order", refundInput("browser-request-0003", 2000, 500))
	if err != nil || last.State != "succeeded" {
		t.Fatalf("remaining: %+v %v", last, err)
	}
	assertLedger(t, db, 2500, 2500, 1)
	if orderStatus(t, db, "refund-order") != "refunded" {
		t.Fatal("full total not reflected")
	}
	if posts, creates := f.counts(); posts != 2 || creates != 2 {
		t.Fatalf("posts=%d creates=%d", posts, creates)
	}
}

func TestRefundRequestTargetsDuplicateReceipt(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "duplicate-selection", 2500, "pi-original")
	if _, err := db.Exec(`UPDATE "order" SET status='fulfilled'; INSERT INTO shipment(order_id,carrier,tracking_number) SELECT id,'carrier','original-tracking' FROM "order"`); err != nil {
		t.Fatal(err)
	}
	if err := svc.markPaid(context.Background(), "stripe", WebhookEvent{ID: "extra-payment", OrderRef: "duplicate-selection", PaymentRef: "pi-refund", AmountCents: 2500, Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	_, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "duplicate-selection", refundInput("refund-extra-payment", 2500, 0))
	if err != nil || r.State != "succeeded" {
		t.Fatalf("refund selected excess payment: %+v %v", r, err)
	}
	assertLedger(t, db, 5000, 2500, 2)
	var originalRefunded int64
	var primary, tracking string
	if err := db.QueryRow(`SELECT refunded_cents FROM payment_receipt WHERE payment_ref='pi-original'`).Scan(&originalRefunded); err != nil || originalRefunded != 0 {
		t.Fatalf("original payment changed: %d %v", originalRefunded, err)
	}
	if err := db.QueryRow(`SELECT payment_ref FROM "order"`).Scan(&primary); err != nil || primary != "pi-original" {
		t.Fatalf("primary payment replaced: %q %v", primary, err)
	}
	if err := db.QueryRow(`SELECT tracking_number FROM shipment`).Scan(&tracking); err != nil || tracking != "original-tracking" || orderStatus(t, db, "duplicate-selection") != "fulfilled" {
		t.Fatalf("fulfillment lost: %q %v", tracking, err)
	}
}

func TestRefundRequestMismatchedResultRequiresReview(t *testing.T) {
	for _, field := range []string{"payment", "currency", "amount", "request", "cumulative"} {
		t.Run(field, func(t *testing.T) {
			svc, db, _ := newTestSvc(t)
			seedPaidOrder(t, db, "mismatch", 2500, "pi-refund")
			a, err := svc.claimRefund(context.Background(), "mismatch", refundInput("mismatch-request-001", 500, 0), "")
			if err != nil {
				t.Fatal(err)
			}
			r := gatewayRefund{ID: "re-mismatch", State: "succeeded", PaymentRef: a.PaymentRef, Currency: a.Currency, RequestID: a.ID, Amount: 500, TotalRefunded: 500}
			switch field {
			case "payment":
				r.PaymentRef = "another-payment"
			case "currency":
				r.Currency = "EUR"
			case "amount":
				r.Amount = 600
			case "request":
				r.RequestID = "another-request"
			case "cumulative":
				r.TotalRefunded = 3000
			}
			result, err := svc.saveRefundResult(context.Background(), a, r, nil)
			if err != nil || result.State != "review" || result.Completed {
				t.Fatalf("mismatch accepted or not persisted: %+v %v", result, err)
			}
			assertLedger(t, db, 2500, 0, 1)
			if _, err := svc.RequestRefund(context.Background(), "mismatch", refundInput("mismatch-request-002", 500, 0)); err != ErrRefundBusy {
				t.Fatalf("unresolved result allowed new request: %v", err)
			}
			var lease int64
			if err := db.QueryRow(`SELECT lease_until FROM refund_request`).Scan(&lease); err != nil || lease != 0 {
				t.Fatalf("mismatch retained stale lease: %d %v", lease, err)
			}
		})
	}
}

func TestRefundRequestUnknownResultSurvivesReopen(t *testing.T) {
	svc, db, kek := newTestSvc(t)
	seedPaidOrder(t, db, "reopen", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.loseFirst = true
	svc.stripe.apiBase = ts.URL
	in := refundInput("browser-reopen-0001", 500, 0)
	first, err := svc.RequestRefund(context.Background(), "reopen", in)
	if err != nil || first.State != "pending" {
		t.Fatalf("lost response: %+v %v", first, err)
	}
	assertLedger(t, db, 2500, 0, 1)
	var seq int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	reopened.SetMaxOpenConns(1)
	cfg := settings.New(reopened)
	keys := NewKeyCache(cfg)
	if err := keys.Unlock(context.Background(), kek); err != nil {
		t.Fatal(err)
	}
	restart := NewService(reopened, cfg, keys)
	restart.stripe.apiBase = ts.URL
	got, err := restart.RetryRefund(context.Background(), "reopen", first.ID, "")
	if err != nil || got.State != "succeeded" {
		t.Fatalf("recover: %+v %v", got, err)
	}
	assertLedger(t, reopened, 2500, 500, 1)
	if posts, creates := f.counts(); posts != 2 || creates != 1 {
		t.Fatalf("posts=%d creates=%d", posts, creates)
	}
}

func TestRefundRequestPendingThenFailedAllowsNewIntent(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "pending", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.initialState = "pending"
	svc.stripe.apiBase = ts.URL
	first, err := svc.RequestRefund(context.Background(), "pending", refundInput("browser-pending-01", 500, 0))
	if err != nil || first.State != "pending" {
		t.Fatalf("pending: %+v %v", first, err)
	}
	assertLedger(t, db, 2500, 0, 1)
	if err := order.New(db, svc.settings).Fulfill(context.Background(), "pending", "carrier", "track", ""); err != sql.ErrNoRows {
		t.Fatalf("shipped while refund pending: %v", err)
	}
	if _, err := svc.RequestRefund(context.Background(), "pending", refundInput("browser-pending-02", 500, 0)); err != ErrRefundBusy {
		t.Fatalf("two active refunds: %v", err)
	}
	f.state(first.ID, "failed")
	failed, err := svc.RetryRefund(context.Background(), "pending", first.ID, "")
	if err != nil || failed.State != "failed" || failed.Completed {
		t.Fatalf("failed: %+v %v", failed, err)
	}
	assertLedger(t, db, 2500, 0, 1)
	if posts, creates := f.counts(); posts != 1 || creates != 1 {
		t.Fatalf("lookup resubmitted refund: %d %d", posts, creates)
	}
	f.mu.Lock()
	f.initialState = "succeeded"
	f.mu.Unlock()
	next, err := svc.RequestRefund(context.Background(), "pending", refundInput("browser-pending-02", 500, 0))
	if err != nil || next.State != "succeeded" {
		t.Fatalf("new intent: %+v %v", next, err)
	}
	assertLedger(t, db, 2500, 500, 1)
}

func TestRefundRequestConcurrencyAndOwnership(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "parallel-refund", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	f.entered, f.release = make(chan struct{}, 1), make(chan struct{})
	in := refundInput("browser-concurrent-01", 500, 0)
	done := make(chan error, 1)
	go func() { _, err := svc.RequestRefund(context.Background(), "parallel-refund", in); done <- err }()
	<-f.entered
	for range 10 {
		independent := NewService(db, svc.settings, svc.keys)
		if _, err := independent.RequestRefund(context.Background(), "parallel-refund", in); err != ErrRefundBusy {
			t.Errorf("concurrent request: %v", err)
		}
	}
	close(f.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	seedPaidOrder(t, db, "different-order", 2500, "pi-other")
	if _, err := svc.RequestRefund(context.Background(), "different-order", in); err != ErrMismatch {
		t.Fatalf("cross-order reference accepted: %v", err)
	}
	var id string
	if err := db.QueryRow(`SELECT public_id FROM refund_request`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RetryRefund(context.Background(), "different-order", id, ""); err != sql.ErrNoRows {
		t.Fatalf("cross-order retry: %v", err)
	}
	if posts, creates := f.counts(); posts != 1 || creates != 1 {
		t.Fatalf("duplicate gateway request: %d %d", posts, creates)
	}
}

func TestRefundRequestOldUnknownOnlyAcceptsVerifiedLookup(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "old", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.loseFirst = true
	svc.stripe.apiBase = ts.URL
	first, err := svc.RequestRefund(context.Background(), "old", refundInput("browser-old-refund-01", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE refund_request SET first_sent_at=?`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	result, err := svc.RetryRefund(context.Background(), "old", first.ID, "")
	if err != nil || result.State != "review" {
		t.Fatalf("must not resubmit old unknown: %+v %v", result, err)
	}
	result, err = svc.RetryRefund(context.Background(), "old", first.ID, "re-1")
	if err != nil || result.State != "succeeded" {
		t.Fatalf("verified manual lookup: %+v %v", result, err)
	}
	if posts, _ := f.counts(); posts != 1 {
		t.Fatalf("unsafe resubmission count=%d", posts)
	}
	assertLedger(t, db, 2500, 500, 1)
}

func TestRefundRequestWebhookBeforeResponseDoesNotDoubleCount(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "race", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	f.callback = func(id string, amount int64) {
		if err := deliverStripe(t, svc, stripeRefund("evt-before-response", "pi-refund", 2500, amount, "usd")); err != nil {
			t.Error(err)
		}
	}
	r, err := svc.RequestRefund(context.Background(), "race", refundInput("browser-race-refund", 500, 0))
	if err != nil || r.State != "succeeded" {
		t.Fatalf("race: %+v %v", r, err)
	}
	assertLedger(t, db, 2500, 500, 1)
	if refundCount(t, db) != 1 {
		t.Fatal("refund record duplicated")
	}
}

func TestRefundRequestSavesAfterBrowserCancellation(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "cancel-client", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.callback = func(string, int64) { cancel() }
	first, err := svc.RequestRefund(ctx, "cancel-client", refundInput("browser-cancel-0001", 500, 0))
	if err != nil || first.ID == "" || first.State != "pending" {
		t.Fatalf("cancelled client lost intent: %+v %v", first, err)
	}
	result, err := svc.RetryRefund(context.Background(), "cancel-client", first.ID, "")
	if err != nil || result.State != "succeeded" {
		t.Fatalf("cancelled recovery: %+v %v", result, err)
	}
	assertLedger(t, db, 2500, 500, 1)
	if _, creates := f.counts(); creates != 1 {
		t.Fatalf("new refund issued after cancellation: %d", creates)
	}
}

func TestRefundRequestConfigurationChangeStopsRecovery(t *testing.T) {
	svc, db, kek := newTestSvc(t)
	seedPaidOrder(t, db, "config-refund", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.loseFirst = true
	svc.stripe.apiBase = ts.URL
	first, err := svc.RequestRefund(context.Background(), "config-refund", refundInput("browser-config-0001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.settings.SetEncrypted(context.Background(), KeyStripeSecret, []byte("sk_test_new_account"), kek); err != nil {
		t.Fatal(err)
	}
	if err := svc.keys.Unlock(context.Background(), kek); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RetryRefund(context.Background(), "config-refund", first.ID, ""); err != ErrPaymentConfigChanged {
		t.Fatalf("changed credentials used: %v", err)
	}
	if posts, _ := f.counts(); posts != 1 {
		t.Fatal("called gateway with changed credentials")
	}
}

func TestPayPalRefundRequestUsesExplicitAmountAndCumulativeResult(t *testing.T) {
	var mu sync.Mutex
	var requestID string
	completed, posts := false, 0
	svc, db, _ := newPayPalSvc(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		mu.Lock()
		if r.Method == http.MethodPost {
			var in struct {
				Amount    paypalMoney `json:"amount"`
				RequestID string      `json:"custom_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Amount.Value != "20.00" || in.Amount.Currency != "USD" || in.RequestID != r.Header.Get("PayPal-Request-Id") || r.Header.Get("Prefer") != "return=representation" {
				mu.Unlock()
				w.WriteHeader(400)
				return
			}
			posts++
			requestID = in.RequestID
		}
		state := "PENDING"
		if completed {
			state = "COMPLETED"
		}
		id := requestID
		mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"id":"RF-PARTIAL","status":%q,"amount":{"value":"20.00","currency_code":"USD"},"custom_id":%q,"seller_payable_breakdown":{"total_refunded_amount":{"value":"30.00","currency_code":"USD"}},"links":[{"rel":"up","href":"https://api.paypal.com/v2/payments/captures/CAP1"}]}`, state, id)
	})
	seedPaidPayPal(t, db, "paypal-refund", 9900, "CAP1")
	first, err := svc.RequestRefund(context.Background(), "paypal-refund", RefundInput{ClientKey: "browser-paypal-001", Provider: "paypal", PaymentRef: "CAP1", AmountCents: 2000})
	if err != nil || first.State != "pending" {
		t.Fatalf("pending paypal refund: %+v %v", first, err)
	}
	assertLedger(t, db, 9900, 0, 1)
	mu.Lock()
	completed = true
	mu.Unlock()
	result, err := svc.RetryRefund(context.Background(), "paypal-refund", first.ID, "")
	if err != nil || result.State != "succeeded" {
		t.Fatalf("paypal result: %+v %v", result, err)
	}
	assertLedger(t, db, 9900, 3000, 1)
	// An older external refund is already included in the cumulative provider
	// total. Its late webhook must not add its amount a second time.
	if err := svc.markRefundedFromWebhook(context.Background(), "paypal", WebhookEvent{ID: "external", Type: "PAYMENT.CAPTURE.REFUNDED", PaymentRef: "CAP1", RefundRef: "RF-EXTERNAL", AmountCents: 1000, RefundedCents: 1000, Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 9900, 3000, 1)
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 {
		t.Fatalf("PayPal refund resubmitted instead of looked up: %d", posts)
	}
}
