// 跨通道支付测试 / Checkout Switching Tests
// 功能：模拟关闭、审批、扣款和响应丢失，验证跨通道并发与原会话恢复边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:58:12
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"
)

type switchSession struct {
	ID, Provider, OrderID, Currency, State string
	Amount                                 int64
	Paid, Pending, Declined                bool
}
type switchFake struct {
	captureKeys                                         map[string]string
	captureEntered, captureRelease                      chan struct{}
	mu                                                  sync.Mutex
	sessions                                            map[string]*switchSession
	keys                                                map[string]string
	creates, expires, captures                          int
	loseExpire, loseCapture, unknownCapture, loseCreate bool
	entered, release                                    chan struct{}
}

func newSwitchFake(t *testing.T) (*switchFake, *httptest.Server) {
	t.Helper()
	f := &switchFake{captureKeys: make(map[string]string), sessions: make(map[string]*switchSession), keys: make(map[string]string)}
	ts := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(ts.Close)
	return f, ts
}
func (f *switchFake) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/v1/oauth2/token" {
		_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
		return
	}
	if r.Method == http.MethodPost && (r.URL.Path == "/v1/checkout/sessions" || r.URL.Path == "/v2/checkout/orders") {
		provider, key := "stripe", r.Header.Get("Idempotency-Key")
		if strings.HasPrefix(r.URL.Path, "/v2/") {
			provider, key = "paypal", r.Header.Get("PayPal-Request-Id")
		}
		id := f.keys[key]
		if id == "" {
			f.creates++
			id = fmt.Sprintf("%s_%d", provider, f.creates)
			row := &switchSession{ID: id, Provider: provider, State: "open"}
			if provider == "stripe" {
				if err := r.ParseForm(); err != nil {
					w.WriteHeader(400)
					return
				}
				row.OrderID = r.Form.Get("client_reference_id")
				row.Currency = r.Form.Get("line_items[0][price_data][currency]")
				var n int64
				if _, err := fmt.Sscan(r.Form.Get("line_items[0][price_data][unit_amount]"), &n); err != nil {
					w.WriteHeader(400)
					return
				}
				row.Amount = n
			} else {
				var in struct {
					Units []struct {
						Custom string `json:"custom_id"`
						Amount paypalMoney
					} `json:"purchase_units"`
				}
				if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Units) != 1 {
					w.WriteHeader(400)
					return
				}
				row.OrderID = in.Units[0].Custom
				row.Currency = in.Units[0].Amount.Currency
				row.Amount, _ = decimalToCents(in.Units[0].Amount.Value)
			}
			f.sessions[id] = row
			f.keys[key] = id
		}
		if f.loseCreate {
			f.loseCreate = false
			w.WriteHeader(503)
			return
		}
		if f.entered != nil {
			select {
			case f.entered <- struct{}{}:
			default:
			}
			<-f.release
		}
		if provider == "stripe" {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "url": "https://checkout.stripe.test/" + id})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "links": []map[string]string{{"rel": "approve", "href": "https://paypal.test/" + id}}})
		}
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		w.WriteHeader(404)
		return
	}
	id := parts[3]
	row := f.sessions[id]
	if row == nil {
		w.WriteHeader(404)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/expire") {
		f.expires++
		if row.State != "open" {
			w.WriteHeader(409)
			return
		}
		row.State = "expired"
		if f.loseExpire {
			w.WriteHeader(503)
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/capture") {
		key := r.Header.Get("PayPal-Request-Id")
		if key == "" || f.captureKeys[id] != "" && f.captureKeys[id] != key {
			w.WriteHeader(400)
			return
		}
		f.captureKeys[id] = key
		f.captures++
		if f.captureEntered != nil {
			f.captureEntered <- struct{}{}
			<-f.captureRelease
		}
		if f.unknownCapture {
			w.WriteHeader(503)
			return
		}
		row.Paid = true
		if f.loseCapture {
			w.WriteHeader(503)
			return
		}
	}
	if row.Provider == "stripe" {
		status, pay := row.State, "unpaid"
		if row.Paid {
			status, pay = "complete", "paid"
		} else if row.Pending {
			status = "complete"
		}
		ref := ""
		if row.Paid || row.Pending {
			ref = "pi_" + row.ID
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": row.ID, "status": status, "payment_status": pay, "client_reference_id": row.OrderID, "amount_total": row.Amount, "currency": row.Currency, "payment_intent": ref})
		return
	}
	unit := map[string]any{"custom_id": row.OrderID, "amount": paypalMoney{Value: centsToDecimal(row.Amount), Currency: row.Currency}}
	status := "APPROVED"
	if row.Paid || row.Pending || row.Declined {
		status = "COMPLETED"
		capStatus := "COMPLETED"
		if row.Pending {
			capStatus = "PENDING"
		}
		if row.Declined {
			capStatus = "DECLINED"
		}
		unit["payments"] = map[string]any{"captures": []any{map[string]any{"id": "CAP_" + row.ID, "status": capStatus, "custom_id": row.OrderID, "amount": unit["amount"]}}}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"id": row.ID, "intent": "CAPTURE", "status": status, "purchase_units": []any{unit}})
}
func (f *switchFake) set(id string, fn func(*switchSession)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.sessions[id])
}
func prepareSwitchService(t *testing.T) (*Service, *switchFake) {
	t.Helper()
	svc, db, _ := newTestSvc(t)
	svc.keys.paypalEnv = resolvePayPalEnv(fakeEnv(map[string]string{envPayPalClientID: "id_x", envPayPalSecret: "sec_x"}))
	f, ts := newSwitchFake(t)
	svc.stripe.apiBase = ts.URL
	svc.paypal.apiBase = ts.URL
	seedOrder(t, db, "switch-order", 2500, "USD")
	return svc, f
}
func switchStart(t *testing.T, svc *Service, provider string) string {
	t.Helper()
	raw, err := svc.StartCheckout(context.Background(), provider, checkoutInput("switch-order"))
	if err != nil {
		t.Fatal(err)
	}
	return raw[strings.LastIndex(raw, "/")+1:]
}

func TestCheckoutSwitchClosesStripeAndRetiresPayPal(t *testing.T) {
	svc, f := prepareSwitchService(t)
	stripeID := switchStart(t, svc, "stripe")
	f.loseExpire = true
	paypalID := switchStart(t, svc, "paypal")
	if f.sessions[stripeID].State != "expired" || f.expires != 1 {
		t.Fatal("old Stripe session remains payable")
	}
	nextStripe := switchStart(t, svc, "stripe")
	if nextStripe == stripeID {
		t.Fatal("expired Stripe session reused")
	}
	if _, err := svc.CapturePayPal(context.Background(), paypalID); err != ErrCheckoutReplaced {
		t.Fatalf("old PayPal return accepted: %v", err)
	}
	if f.captures != 0 {
		t.Fatal("retired PayPal caused capture")
	}
	if active := switchStart(t, svc, "stripe"); active != nextStripe {
		t.Fatal("same method lost idempotency")
	}
	var n int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM payment_attempt WHERE state='retired'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("history %d %v", n, err)
	}
	// The migration may be reapplied without losing retired attempts or captures.
	raw, err := migrations.FS.ReadFile("0033_checkout_switch.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = migrate.Run(context.Background(), svc.db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = svc.db.QueryRow(`SELECT COUNT(*) FROM payment_attempt WHERE state='retired'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("history lost on rerun: %d %v", n, err)
	}
}

func TestCheckoutSwitchBlocksConfirmedAndPendingStripe(t *testing.T) {
	for _, paid := range []bool{true, false} {
		t.Run(fmt.Sprint(paid), func(t *testing.T) {
			svc, f := prepareSwitchService(t)
			id := switchStart(t, svc, "stripe")
			f.set(id, func(s *switchSession) { s.Paid = paid; s.Pending = !paid })
			if _, err := svc.StartCheckout(context.Background(), "paypal", checkoutInput("switch-order")); err != ErrPaymentProcessing {
				t.Fatalf("processing bypass: %v", err)
			}
			if f.creates != 1 {
				t.Fatal("created second payable checkout")
			}
			want := "pending"
			if paid {
				want = "paid"
			}
			if got := orderStatus(t, svc.db, "switch-order"); got != want {
				t.Fatalf("status %s want %s", got, want)
			}
		})
	}
}

func TestPayPalCaptureLostResponseRecoveredBeforeSwitch(t *testing.T) {
	svc, f := prepareSwitchService(t)
	id := switchStart(t, svc, "paypal")
	f.loseCapture = true
	if _, err := svc.CapturePayPal(context.Background(), id); err == nil {
		t.Fatal("lost response accepted")
	}
	if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("switch-order")); err != ErrPaymentProcessing {
		t.Fatalf("did not reconcile capture before switch: %v", err)
	}
	if orderStatus(t, svc.db, "switch-order") != "paid" || f.creates != 1 {
		t.Fatal("paid receipt lost or second checkout created")
	}
	if _, err := svc.CapturePayPal(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if f.captures != 1 {
		t.Fatal("repeat return captured again")
	}
}

func TestPayPalUnknownCaptureBlocksSwitchAndExpiredResend(t *testing.T) {
	svc, f := prepareSwitchService(t)
	id := switchStart(t, svc, "paypal")
	f.unknownCapture = true
	if _, err := svc.CapturePayPal(context.Background(), id); err == nil {
		t.Fatal("unknown capture accepted")
	}
	if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("switch-order")); err != ErrPaymentProcessing {
		t.Fatalf("unknown capture switched: %v", err)
	}
	if _, err := svc.db.Exec(`UPDATE checkout_capture SET first_sent_at=?`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CapturePayPal(context.Background(), id); err != ErrPaymentProcessing {
		t.Fatalf("expired key re-submitted: %v", err)
	}
	if f.captures != 1 || f.creates != 1 {
		t.Fatal("new money request after unknown result")
	}
	f.set(id, func(s *switchSession) { s.Declined = true })
	switchStart(t, svc, "stripe")
	if f.creates != 2 {
		t.Fatal("definitive declined capture did not allow switch")
	}
}

func TestCheckoutOrderLockSerializesDifferentProviders(t *testing.T) {
	svc, f := prepareSwitchService(t)
	f.entered = make(chan struct{}, 1)
	f.release = make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("switch-order"))
		done <- err
	}()
	<-f.entered
	if _, err := svc.StartCheckout(context.Background(), "paypal", checkoutInput("switch-order")); err != ErrPaymentInProgress {
		t.Fatalf("parallel provider bypass: %v", err)
	}
	close(f.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCheckoutSwitchRecoversUnknownCreateUsingOriginalKey(t *testing.T) {
	svc, f := prepareSwitchService(t)
	f.loseCreate = true
	if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("switch-order")); err == nil {
		t.Fatal("missing response accepted")
	}
	var key string
	if err := svc.db.QueryRow(`SELECT public_id FROM payment_attempt`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	switchStart(t, svc, "paypal")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.creates != 2 || f.expires != 1 || f.sessions[f.keys[key]].State != "expired" {
		t.Fatal("original unknown session not closed before new provider")
	}
}

func TestCheckoutSwitchAndCaptureShareLease(t *testing.T) {
	svc, f := prepareSwitchService(t)
	id := switchStart(t, svc, "paypal")
	f.captureEntered = make(chan struct{}, 1)
	f.captureRelease = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := svc.CapturePayPal(context.Background(), id); done <- err }()
	<-f.captureEntered
	if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("switch-order")); err != ErrPaymentInProgress {
		t.Fatalf("switch raced capture: %v", err)
	}
	close(f.captureRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if orderStatus(t, svc.db, "switch-order") != "paid" {
		t.Fatal("capture not recorded")
	}
}

func TestCheckoutLockRecoveryAndSessionMismatch(t *testing.T) {
	svc, f := prepareSwitchService(t)
	id := switchStart(t, svc, "stripe")
	if _, err := svc.db.Exec(`UPDATE checkout_lock SET lease_token='crashed',lease_until=?`, time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	f.set(id, func(s *switchSession) { s.OrderID = "another-order" })
	if _, err := svc.StartCheckout(context.Background(), "paypal", checkoutInput("switch-order")); err != ErrMismatch {
		t.Fatalf("unrelated session expired: %v", err)
	}
	if f.expires != 0 || f.creates != 1 {
		t.Fatal("gateway mutated after mismatch")
	}
	f.set(id, func(s *switchSession) { s.OrderID = "switch-order" })
	switchStart(t, svc, "paypal")
}

func TestPayPalCaptureRejectsUnknownTokenAndExpiredOrder(t *testing.T) {
	svc, f := prepareSwitchService(t)
	for _, id := range []string{"", "../order", "https://other.example", "PP-unknown"} {
		if _, err := svc.CapturePayPal(context.Background(), id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	id := switchStart(t, svc, "paypal")
	if _, err := svc.db.Exec(`UPDATE "order" SET reservation_expires_at=?`, time.Now().Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CapturePayPal(context.Background(), id); err != ErrOrderExpired {
		t.Fatalf("expired capture: %v", err)
	}
	if f.captures != 0 {
		t.Fatal("expired order was charged")
	}
	// Actual money received outside the return path must still be recorded as late.
	f.set(id, func(s *switchSession) { s.Paid = true })
	if _, err := svc.CapturePayPal(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if orderStatus(t, svc.db, "switch-order") != "payment_review" {
		t.Fatal("late payment was allowed to fulfill")
	}
}

func TestPayPalUnknownCaptureRetriesSameKeyWithinWindow(t *testing.T) {
	svc, f := prepareSwitchService(t)
	id := switchStart(t, svc, "paypal")
	f.unknownCapture = true
	if _, err := svc.CapturePayPal(context.Background(), id); err == nil {
		t.Fatal("unknown capture accepted")
	}
	raw, err := migrations.FS.ReadFile("0033_checkout_switch.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = svc.db.QueryRow(`SELECT state FROM checkout_capture`).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("migration erased unknown result: %s %v", state, err)
	}
	f.unknownCapture = false
	if _, err = svc.CapturePayPal(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if f.captures != 2 || orderStatus(t, svc.db, "switch-order") != "paid" {
		t.Fatal("same-key capture recovery failed")
	}
}
