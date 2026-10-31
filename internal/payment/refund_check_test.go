// 退款校正测试 / Refund Reconciliation Tests
// 功能：验证完整快照、失败后金额校正、后续退款与旧通知、未知结果和对象边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:43:04
package payment

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/order"
)

func TestRefundCheckCorrectsFailureAndHandlesLaterRefund(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "check-failed", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "check-failed", refundInput("check-failed-001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	f.state(r.ID, "failed")
	if err = deliverStripe(t, svc, refundStatusEvent("failed", "refund.failed", r.ID, "pi-refund")); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ReconcileRefunds(context.Background(), "check-failed", "stripe", "pi-refund")
	if err != nil || result.RefundedCents != 0 || result.Pending {
		t.Fatalf("check: %+v %v", result, err)
	}
	assertLedger(t, db, 2500, 0, 1)
	var state string
	var issues, history int
	if err = db.QueryRow(`SELECT state FROM refund_request`).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("state %s %v", state, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM refund_reconciliation WHERE state='open'`).Scan(&issues); err != nil || issues != 0 {
		t.Fatalf("issues %d %v", issues, err)
	}
	if orderStatus(t, db, "check-failed") != "paid" || refundCount(t, db) != 1 {
		t.Fatal("correction must preserve posting history")
	}
	if err = deliverStripe(t, svc, stripeRefund("old-cumulative", "pi-refund", 2500, 500, "usd")); err != nil {
		t.Fatal(err)
	}
	assertLedger(t, db, 2500, 0, 1)
	// A new refund after correction must not count the failed historical posting.
	next, err := svc.RequestRefund(context.Background(), "check-failed", refundInput("check-failed-002", 700, 0))
	if err != nil || next.State != "succeeded" {
		t.Fatalf("new refund: %+v %v", next, err)
	}
	assertLedger(t, db, 2500, 700, 1)
	if posts, _ := f.counts(); posts != 2 {
		t.Fatalf("unexpected refund POST count %d", posts)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM refund_check`).Scan(&history); err != nil || history < 3 {
		t.Fatalf("history %d %v", history, err)
	}
	o, err := order.New(db, svc.settings).AdminGet(context.Background(), "check-failed")
	if err != nil || o.Receipts[0].RefundCheckedAt == "" || o.RefundStates["stripe:re-1"] != "failed" {
		t.Fatalf("view %+v %v", o, err)
	}
}

func TestRefundCheckPendingAndUnknownRecovery(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "check-pending", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.initialState = "pending"
	f.loseFirst = true
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "check-pending", refundInput("check-pending-001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE refund_request SET state='review',reason='unknown_result',first_sent_at=?`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReconcileRefunds(context.Background(), "check-pending", "stripe", "pi-refund")
	if err != nil || !got.Pending || got.RefundedCents != 0 {
		t.Fatalf("pending %+v %v", got, err)
	}
	if _, err = svc.RequestRefund(context.Background(), "check-pending", refundInput("check-pending-002", 300, 0)); err != ErrRefundConflict {
		t.Fatalf("pending bypass %v", err)
	}
	f.state(r.ID, "succeeded")
	result, err := svc.RetryRefund(context.Background(), "check-pending", r.ID, "")
	if err != nil || result.State != "succeeded" {
		t.Fatalf("worker/manual recovery %+v %v", result, err)
	}
	assertLedger(t, db, 2500, 500, 1)
	if posts, _ := f.counts(); posts != 1 {
		t.Fatal("check sent new refund")
	}
}

func TestRefundCheckRejectsMissingRecordsAndCrossOrder(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "check-missing", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "check-missing", refundInput("check-missing-001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	delete(f.records, r.ID)
	f.mu.Unlock()
	if _, err = svc.ReconcileRefunds(context.Background(), "check-missing", "stripe", "pi-refund"); err != ErrRefundUnconfirmed {
		t.Fatalf("missing silently erased: %v", err)
	}
	assertLedger(t, db, 2500, 500, 1)
	seedPaidOrder(t, db, "other-order", 2500, "pi-other")
	if _, err = svc.ReconcileRefunds(context.Background(), "other-order", "stripe", "pi-refund"); err != ErrMismatch {
		t.Fatalf("cross order %v", err)
	}
	if _, err = svc.ReconcileRefunds(context.Background(), "not-an-order", "stripe", "pi-refund"); err != sql.ErrNoRows {
		t.Fatalf("missing order %v", err)
	}
}

func TestRefundCheckRejectsConcurrentLocalChange(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "check-race", 2500, "pi-race")
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/payment_intents/pi-race" {
			_, _ = w.Write([]byte(`{"id":"pi-race","currency":"usd","amount_received":2500,"status":"succeeded"}`))
			return
		}
		if calls.Add(1) == 1 {
			// The network read must not hold the only SQLite connection.
			if _, err := db.Exec(`UPDATE payment_receipt SET refunded_cents=100`); err != nil {
				t.Error(err)
			}
		}
		_, _ = w.Write([]byte(`{"object":"list","has_more":false,"data":[]}`))
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	if _, err := svc.ReconcileRefunds(context.Background(), "check-race", "stripe", "pi-race"); err != ErrRefundBusy {
		t.Fatalf("stale snapshot committed: %v", err)
	}
	assertLedger(t, db, 2500, 100, 1)
}

func TestStripeRefundInventoryPaginationAndMalformedResponses(t *testing.T) {
	for _, mode := range []string{"valid", "duplicate", "truncated", "wrong-payment", "over-total", "unknown-state", "missing-more"} {
		t.Run(mode, func(t *testing.T) {
			svc, _, _ := newTestSvc(t)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("inventory attempted write")
				}
				if r.URL.Path == "/v1/payment_intents/pi-list" {
					_, _ = w.Write([]byte(`{"id":"pi-list","currency":"usd","amount_received":2500,"status":"succeeded"}`))
					return
				}
				if r.URL.Query().Get("payment_intent") != "pi-list" || r.URL.Query().Get("limit") != "100" {
					t.Error("missing payment scope")
				}
				id := "r1"
				more := "true"
				amount := 500
				ref := "pi-list"
				state := "succeeded"
				if r.URL.Query().Get("starting_after") != "" {
					id = "r2"
					more = "false"
				}
				switch mode {
				case "duplicate":
					id = "r1"
				case "truncated":
					_, _ = w.Write([]byte(`{"object":"list","has_more":true,"data":[]}`))
					return
				case "wrong-payment":
					ref = "pi-other"
				case "over-total":
					amount = 2000
				case "unknown-state":
					state = "mystery"
				case "missing-more":
					_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
					return
				}
				_, _ = fmt.Fprintf(w, `{"object":"list","has_more":%s,"data":[{"id":%q,"amount":%d,"currency":"usd","payment_intent":%q,"status":%q}]}`, more, id, amount, ref, state)
			}))
			defer ts.Close()
			svc.stripe.apiBase = ts.URL
			got, err := svc.refundInventory(context.Background(), "stripe", "pi-list", "USD", 2500)
			if mode == "valid" {
				if err != nil || got.Total != 1000 || len(got.Refunds) != 2 {
					t.Fatalf("inventory %+v %v", got, err)
				}
			} else if err == nil {
				t.Fatal("unsafe inventory accepted")
			}
		})
	}
}

func TestPayPalRefundCheckCorrectsAndRetainsShipment(t *testing.T) {
	var requestID string
	var state atomic.Value
	state.Store("COMPLETED")
	refundJSON := func() string {
		return fmt.Sprintf(`{"id":"PP-check","status":%q,"custom_id":%q,"amount":{"value":"5.00","currency_code":"USD"},"seller_payable_breakdown":{"total_refunded_amount":{"value":"5.00","currency_code":"USD"}},"links":[{"href":"https://api.paypal.com/v2/payments/captures/cap-check","rel":"up"}]}`, state.Load(), requestID)
	}
	svc, db, _ := newPayPalSvc(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
		case "/v2/payments/captures/cap-check/refund":
			requestID = r.Header.Get("PayPal-Request-Id")
			_, _ = w.Write([]byte(refundJSON()))
		case "/v2/payments/captures/cap-check":
			status := "PARTIALLY_REFUNDED"
			if state.Load() == "FAILED" {
				status = "COMPLETED"
			}
			_, _ = fmt.Fprintf(w, `{"id":"cap-check","status":%q,"amount":{"value":"25.00","currency_code":"USD"},"supplementary_data":{"related_ids":{"order_id":"ORDER-CHECK"}}}`, status)
		case "/v2/checkout/orders/ORDER-CHECK":
			_, _ = fmt.Fprintf(w, `{"id":"ORDER-CHECK","purchase_units":[{"payments":{"captures":[{"id":"cap-check","amount":{"value":"25.00","currency_code":"USD"}}],"refunds":[%s]}}]}`, refundJSON())
		default:
			w.WriteHeader(404)
		}
	})
	seedPaidOrder(t, db, "pp-check", 2500, "cap-check")
	if _, err := db.Exec(`UPDATE "order" SET payment_provider='paypal'`); err != nil {
		t.Fatal(err)
	}
	os := order.New(db, svc.settings)
	if err := os.Fulfill(context.Background(), "pp-check", "Carrier", "Tracking", ""); err != nil {
		t.Fatal(err)
	}
	r, err := svc.RequestRefund(context.Background(), "pp-check", RefundInput{ClientKey: "pp-check-request-001", Provider: "paypal", PaymentRef: "cap-check", AmountCents: 500})
	if err != nil || r.State != "succeeded" {
		t.Fatalf("initial %+v %v", r, err)
	}
	state.Store("FAILED")
	result, err := svc.ReconcileRefunds(context.Background(), "pp-check", "paypal", "cap-check")
	if err != nil || result.RefundedCents != 0 {
		t.Fatalf("paypal check %+v %v", result, err)
	}
	assertLedger(t, db, 2500, 0, 1)
	if orderStatus(t, db, "pp-check") != "fulfilled" {
		t.Fatal("shipment lost")
	}
}

func TestPayPalInventoryScopesCaptureAndRejectsMissingHistory(t *testing.T) {
	for _, mode := range []string{"valid", "missing-capture", "missing-order", "missing-history", "wrong-currency", "wrong-amount", "malformed-link"} {
		t.Run(mode, func(t *testing.T) {
			svc, _, _ := newPayPalSvc(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/oauth2/token" {
					_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
					return
				}
				if r.Method != http.MethodGet {
					t.Error("inventory attempted money operation")
				}
				if r.URL.Path == "/v2/payments/captures/CAP" {
					orderID := "ORD"
					if mode == "missing-order" {
						orderID = ""
					}
					_, _ = fmt.Fprintf(w, `{"id":"CAP","status":"PARTIALLY_REFUNDED","amount":{"value":"25.00","currency_code":"USD"},"supplementary_data":{"related_ids":{"order_id":%q}}}`, orderID)
					return
				}
				captureID, currency, amount, link := "CAP", "USD", "5.00", "https://api.paypal.com/v2/payments/captures/CAP"
				if mode == "missing-capture" {
					captureID = "OTHER"
				}
				if mode == "wrong-currency" {
					currency = "EUR"
				}
				if mode == "wrong-amount" {
					amount = "26.00"
				}
				if mode == "malformed-link" {
					link = "https://evil.example/"
				}
				refunds := fmt.Sprintf(`{"id":"R1","status":"COMPLETED","amount":{"value":%q,"currency_code":%q},"links":[{"rel":"up","href":%q}]},{"id":"R2","status":"FAILED","amount":{"value":"2.00","currency_code":"USD"},"links":[{"rel":"up","href":"https://api.paypal.com/v2/payments/captures/CAP"}]},{"id":"OTHER-REFUND","status":"COMPLETED","amount":{"value":"1.00","currency_code":"USD"},"links":[{"rel":"up","href":"https://api.paypal.com/v2/payments/captures/OTHER"}]}`, amount, currency, link)
				if mode == "missing-history" {
					refunds = ""
				}
				_, _ = fmt.Fprintf(w, `{"id":"ORD","purchase_units":[{"payments":{"captures":[{"id":%q,"amount":{"value":"25.00","currency_code":"USD"}},{"id":"OTHER"}],"refunds":[%s]}}]}`, captureID, refunds)
			})
			got, err := svc.refundInventory(context.Background(), "paypal", "CAP", "USD", 2500)
			if mode == "valid" {
				if err != nil || got.Total != 500 || len(got.Refunds) != 2 {
					t.Fatalf("paypal %+v %v", got, err)
				}
			} else if err == nil {
				t.Fatal("unsafe paypal history accepted")
			}
		})
	}
}

func TestRefundCheckRejectsChangingRemoteSnapshot(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "changing", 2500, "pi-change")
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/payment_intents/pi-change" {
			_, _ = w.Write([]byte(`{"id":"pi-change","currency":"usd","amount_received":2500,"status":"succeeded"}`))
			return
		}
		state := "succeeded"
		if calls.Add(1) > 1 {
			state = "failed"
		}
		_, _ = fmt.Fprintf(w, `{"object":"list","has_more":false,"data":[{"id":"r1","amount":500,"currency":"usd","payment_intent":"pi-change","status":%q}]}`, state)
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	if _, err := svc.ReconcileRefunds(context.Background(), "changing", "stripe", "pi-change"); err != ErrRefundBusy {
		t.Fatalf("moving snapshot accepted: %v", err)
	}
	assertLedger(t, db, 2500, 0, 1)
}

func TestTrackedRefundEventWaitsForInventoryVisibility(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "visibility", 2500, "pi-refund")
	_, ts := newRefundServer(t)
	svc.stripe.apiBase = ts.URL
	if _, err := svc.ReconcileRefunds(context.Background(), "visibility", "stripe", "pi-refund"); err != nil {
		t.Fatal(err)
	}
	handled, err := svc.syncTrackedRefundEvent(context.Background(), "stripe", WebhookEvent{ID: "not-yet-listed", Type: "refund.created", PaymentRef: "pi-refund", RefundRef: "re-new"})
	if !handled || err != ErrRefundUnconfirmed {
		t.Fatalf("not visible %+v %v", handled, err)
	}
	if eventCount(t, db) != 0 {
		t.Fatal("not-yet-listed event consumed")
	}
}

func TestRefundCheckLegacyAndLatePaymentPolicies(t *testing.T) {
	for _, status := range []string{"refunded", "payment_review"} {
		t.Run(status, func(t *testing.T) {
			svc, db, _ := newTestSvc(t)
			seedPaidOrder(t, db, "historical", 2500, "pi-refund")
			if _, err := db.Exec(`UPDATE "order" SET status=?;`, status); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO refund(order_id,provider,provider_refund_id,amount_cents) SELECT id,'stripe','re-old',500 FROM "order"`); err != nil {
				t.Fatal(err)
			}
			f, ts := newRefundServer(t)
			svc.stripe.apiBase = ts.URL
			f.records["external"] = fakeRefundRecord{ID: "re-old", Key: "external", State: "failed", Amount: 500}
			if _, err := svc.ReconcileRefunds(context.Background(), "historical", "stripe", "pi-refund"); err != nil {
				t.Fatal(err)
			}
			assertLedger(t, db, 2500, 0, 1)
			want := "paid"
			if status == "payment_review" {
				want = "payment_review"
			}
			if got := orderStatus(t, db, "historical"); got != want {
				t.Fatalf("review policy %s want %s", got, want)
			}
		})
	}
}

func TestStripeRefundInventoryBound(t *testing.T) {
	svc, _, _ := newTestSvc(t)
	var pages atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/payment_intents/pi-limit" {
			_, _ = w.Write([]byte(`{"id":"pi-limit","currency":"usd","amount_received":2500,"status":"succeeded"}`))
			return
		}
		page := pages.Add(1)
		_, _ = w.Write([]byte(`{"object":"list","has_more":true,"data":[`))
		for i := 0; i < 100; i++ {
			if i > 0 {
				_, _ = w.Write([]byte(","))
			}
			_, _ = fmt.Fprintf(w, `{"id":"r%d-%d","amount":1,"currency":"usd","payment_intent":"pi-limit","status":"succeeded"}`, page, i)
		}
		_, _ = w.Write([]byte("]}"))
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	if _, err := svc.refundInventory(context.Background(), "stripe", "pi-limit", "USD", 2500); err != ErrRefundUnconfirmed {
		t.Fatalf("truncated inventory accepted: %v", err)
	}
	if pages.Load() != 10 {
		t.Fatalf("unbounded pagination: %d", pages.Load())
	}
}

func TestRefundCheckTrackedBrowserCancellation(t *testing.T) {
	for _, test := range []struct {
		name           string
		knownReference bool
		posts, lookups int
	}{
		{name: "cancel_before_refund_result", posts: 2},
		{name: "cancel_after_refund_id", knownReference: true, posts: 1, lookups: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, db, _ := newTestSvc(t)
			seedPaidOrder(t, db, "tracked-cancel", 2500, "pi-refund")
			f, ts := newRefundServer(t)
			svc.stripe.apiBase = ts.URL
			if _, err := svc.ReconcileRefunds(context.Background(), "tracked-cancel", "stripe", "pi-refund"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.knownReference {
				// The refund ID has been decoded before the charge lookup starts.
				// Cancel at this exact boundary, rather than race the HTTP response.
				base := svc.stripe.httpClient.Transport
				if base == nil {
					base = http.DefaultTransport
				}
				var interrupted atomic.Bool
				svc.stripe.httpClient.Transport = attemptRoundTripper(func(request *http.Request) (*http.Response, error) {
					if request.URL.Path == "/v1/charges/ch-refund" && interrupted.CompareAndSwap(false, true) {
						cancel()
						return nil, context.Canceled
					}
					return base.RoundTrip(request)
				})
			} else {
				// The first response contains no refund ID even if the transport
				// manages to read it after cancellation. The gateway still created it.
				f.loseFirst = true
				f.callback = func(string, int64) { cancel() }
			}
			first, err := svc.RequestRefund(ctx, "tracked-cancel", refundInput("tracked-cancel-001", 500, 0))
			if err != nil || first.ID == "" || first.State != "review" || ctx.Err() != context.Canceled {
				t.Fatalf("cancel lost request: %+v %v", first, err)
			}
			var reference string
			if err = db.QueryRow(`SELECT gateway_ref FROM refund_request WHERE public_id=?`, first.ID).Scan(&reference); err != nil {
				t.Fatal(err)
			}
			wantReference := ""
			if test.knownReference {
				wantReference = "re-1"
			}
			if reference != wantReference {
				t.Fatalf("cancellation boundary reference=%q want%q", reference, wantReference)
			}
			assertLedger(t, db, 2500, 0, 1)
			next, err := svc.RetryRefund(context.Background(), "tracked-cancel", first.ID, "")
			if err != nil || next.ID != first.ID || next.State != "succeeded" {
				t.Fatalf("recovery %+v %v", next, err)
			}
			assertLedger(t, db, 2500, 500, 1)
			if posts, created := f.counts(); posts != test.posts || created != 1 {
				t.Fatalf("gateway writes posts=%d want%d created=%d want1", posts, test.posts, created)
			}
			f.mu.Lock()
			lookups := f.lookups
			f.mu.Unlock()
			if lookups != test.lookups {
				t.Fatalf("read-only refund lookups=%d want%d", lookups, test.lookups)
			}
			var requests int
			var key string
			if err = db.QueryRow(`SELECT count(*),min(client_key) FROM refund_request`).Scan(&requests, &key); err != nil || requests != 1 || key != "tracked-cancel-001" {
				t.Fatalf("original durable request changed: count%d key%q err%v", requests, key, err)
			}
		})
	}
}
