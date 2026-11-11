// 支付到通知贯通回归 / Verified Payment to Shipment Delivery Flow
// 功能：真实验签回调、原子库存/确认信、发货版本、本机SMTP未知投递及已发货退款的跨模块隔离验证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 20:39:31
package admin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/storefront"
)

func deliveryWait(t *testing.T, check func(context.Context) (bool, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := check(ctx)
		if err != nil {
			t.Fatalf("delivery state query: %v", err)
		}
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("delivery state did not settle within bounded wait")
		case <-ticker.C:
		}
	}
}
func deliveryWorker(t *testing.T, h *HTTP, logs *bytes.Buffer) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	worker := mail.NewWorker(h.svc.db, h.mailCache, slog.New(slog.NewTextHandler(logs, nil)), 5*time.Millisecond)
	go func() { defer close(done); worker.Run(ctx) }()
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(stop)
	return stop
}
func deliverySignedEvent(t *testing.T, mux http.Handler, secret string, event any, badSignature bool, want int) {
	t.Helper()
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := hmac.New(sha256.New, []byte(secret))
	_, _ = signature.Write([]byte(timestamp + "."))
	_, _ = signature.Write(body)
	digest := hex.EncodeToString(signature.Sum(nil))
	if badSignature {
		digest = strings.Repeat("0", 64)
	}
	request := httptest.NewRequest("POST", "https://store.example/webhooks/stripe", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Stripe-Signature", "t="+timestamp+",v1="+digest)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("signed event HTTP status=%d want=%d", response.Code, want)
	}
}
func deliveryPaymentEvent(eventID, eventType, orderID, reference, status string, amount int) any {
	return map[string]any{"id": eventID, "type": eventType, "data": map[string]any{"object": map[string]any{"id": "cs_fixture_delivery", "client_reference_id": orderID, "payment_intent": reference, "payment_status": status, "amount_total": amount, "currency": "usd"}}}
}
func deliveryRefundEvent(eventID, reference string, refunded int) any {
	return map[string]any{"id": eventID, "type": "charge.refunded", "data": map[string]any{"object": map[string]any{"id": "ch_fixture_delivery", "payment_intent": reference, "amount": 1500, "amount_refunded": refunded, "currency": "usd"}}}
}

type deliveryHistory struct {
	CurrentRevision string `json:"current_revision"`
	CurrentVersion  int    `json:"current_version"`
	ShippedAt       string `json:"shipped_at"`
	Notices         []struct {
		ID         string `json:"public_id"`
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
		Version    int    `json:"version"`
	} `json:"notices"`
}

func TestVerifiedPaymentShipmentDeliveryAndRefundFlow(t *testing.T) {
	// All settings come from this test's encrypted database, never the host's accounts.
	for _, name := range []string{"STRIPE_SECRET_KEY", "PAYPAL_CLIENT_ID", "SMTP_HOST"} {
		t.Setenv(name, "")
	}
	h, _ := newHTTP(t)
	db := h.svc.db
	keys := payment.NewKeyCache(h.settings)
	h.svc.SetPaymentKeys(keys)
	payments := payment.NewService(db, h.settings, keys)
	mux := http.NewServeMux()
	h.Register(mux)
	payment.NewHTTP(payments).Register(mux)
	customer := storefront.NewHTTP(storefront.New(db), cart.New(db), h.orders, h.settings, nil, redirect.New(db), "Delivery fixture", "https://store.example", nil)
	customer.Register(mux)
	owner, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{owner}

	// Create a new order through merchant/customer HTTP routes rather than SQL paid seeds.
	created := flowJSON(t, mux, "POST", "/admin/api/products", `{"title":"Delivery Running Tee","slug":"delivery-tee","status":"draft","options":[{"name":"Size","values":["S"]}],"variants":[{"price_cents":1200,"quantity":4,"selections":[{"option":"Size","value":"S"}]}]}`, auth, csrf, 201)
	var product struct {
		ID string `json:"public_id"`
	}
	flowDecode(t, created.Body, &product)
	flowJSON(t, mux, "PATCH", "/admin/api/products/"+product.ID, `{"title":"Delivery Running Tee","status":"active"}`, auth, csrf, 200)
	flowJSON(t, mux, "PUT", "/admin/api/settings/shipping/countries", `{"countries":["US"]}`, auth, csrf, 200)
	flowJSON(t, mux, "PUT", "/admin/api/settings/shipping/default", `{"rate_cents":300,"free_over_cents":0}`, auth, csrf, 200)
	var details struct {
		Variants []struct {
			ID string `json:"public_id"`
		} `json:"variants"`
	}
	flowDecode(t, flowJSON(t, mux, "GET", "/admin/api/products/"+product.ID, "", auth, "", 200).Body, &details)
	if len(details.Variants) != 1 {
		t.Fatal("fixture product variant missing")
	}
	added := flowRequest(t, mux, "POST", "/cart/items", `{"variant":"`+details.Variants[0].ID+`","quantity":1}`, "application/json")
	if added.Code != 200 {
		t.Fatal("fixture add to cart failed")
	}
	cartCookie := flowCookie(t, added.Result().Cookies(), "kartwo_cart")
	form := url.Values{"checkout_action": {"review"}, "country": {"US"}, "email": {"buyer-delivery@example.invalid"}, "name": {"Delivery Fixture"}, "line1": {"12 Fixture Street"}, "city": {"Boston"}, "region": {"MA"}, "postal_code": {"02108"}}
	reviewed := flowRequest(t, mux, "POST", "/checkout", form.Encode(), "application/x-www-form-urlencoded", cartCookie)
	if reviewed.Code != 200 {
		t.Fatal("fixture quote failed")
	}
	form.Set("quote", flowInput(t, reviewed, "quote"))
	form.Set("checkout_action", "place")
	placed := flowRequest(t, mux, "POST", "/checkout", form.Encode(), "application/x-www-form-urlencoded", cartCookie)
	if placed.Code != 303 {
		t.Fatal("fixture checkout failed")
	}
	id := strings.TrimPrefix(placed.Header().Get("Location"), "/order/")
	orderAPI := "/admin/api/orders/" + id
	grant := flowCookie(t, placed.Result().Cookies(), "kartwo_order_access")

	// Configure only a runtime-generated signing secret. No API key or Stripe network request exists.
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	secret := "whsec_" + hex.EncodeToString(random)
	configuration, err := json.Marshal(map[string]any{"stripe": map[string]string{"mode": "test", "webhook_secret": secret}})
	if err != nil {
		t.Fatal(err)
	}
	flowJSON(t, mux, "PUT", "/admin/api/settings/payment", string(configuration), auth, csrf, 200)
	var encrypted int
	var stored string
	if err := db.QueryRow("SELECT encrypted,value FROM setting WHERE key=?", payment.KeyStripeWebhookSecret).Scan(&encrypted, &stored); err != nil || encrypted != 1 || strings.Contains(stored, secret) {
		t.Fatal("webhook secret not encrypted in isolated settings")
	}
	assertState := func(status string, stock, reserved, receipts, emails int) {
		t.Helper()
		var actual string
		var quantity, held, paidRows, outbox int
		if err := db.QueryRow(`SELECT status FROM "order" WHERE public_id=?`, id).Scan(&actual); err != nil || actual != status {
			t.Fatalf("order status=%s want=%s err=%v", actual, status, err)
		}
		if err := db.QueryRow(`SELECT SUM(quantity),SUM(reserved) FROM inventory`).Scan(&quantity, &held); err != nil || quantity != stock || held != reserved {
			t.Fatalf("stock=%d reserved=%d want=%d/%d err=%v", quantity, held, stock, reserved, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM payment_receipt`).Scan(&paidRows); err != nil || paidRows != receipts {
			t.Fatal("receipt count differs", paidRows, receipts, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&outbox); err != nil || outbox != emails {
			t.Fatal("outbox count differs", outbox, emails, err)
		}
	}
	reference := "pi_fixture_" + id
	forged := deliveryPaymentEvent("evt_forged", "checkout.session.completed", id, reference, "paid", 1500)
	deliverySignedEvent(t, mux, secret, forged, true, 400)
	assertState("pending", 4, 1, 0, 0)
	deliverySignedEvent(t, mux, secret, deliveryPaymentEvent("evt_amount_mismatch", "checkout.session.completed", id, reference, "paid", 1501), false, 400)
	assertState("pending", 4, 1, 0, 0)
	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM webhook_event`).Scan(&eventCount); err != nil || eventCount != 0 {
		t.Fatal("rejected events consumed receipt idempotency state")
	}
	deliverySignedEvent(t, mux, secret, deliveryPaymentEvent("evt_unpaid", "checkout.session.completed", id, reference, "unpaid", 1500), false, 200)
	assertState("pending", 4, 1, 0, 0)
	flowJSON(t, mux, "POST", orderAPI+"/fulfill", `{"carrier":"Fixture carrier","tracking_number":"DELIVERY-1"}`, auth, csrf, 409)
	paid := deliveryPaymentEvent("evt_async_paid", "checkout.session.async_payment_succeeded", id, reference, "paid", 1500)
	deliverySignedEvent(t, mux, secret, paid, false, 200)
	assertState("paid", 3, 0, 1, 1)
	deliverySignedEvent(t, mux, secret, paid, false, 200)
	deliverySignedEvent(t, mux, secret, deliveryPaymentEvent("evt_same_payment", "checkout.session.completed", id, reference, "paid", 1500), false, 200)
	assertState("paid", 3, 0, 1, 1)

	// A real SMTP protocol sink, bound to loopback, never forwards any message.
	smtp := newDeliverySMTP(t)
	host, port, err := net.SplitHostPort(smtp.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	smtpConfiguration, err := json.Marshal(map[string]string{"host": host, "port": port, "from_address": "sender-delivery@example.invalid", "from_name": "Isolated fixture", "encryption": "none"})
	if err != nil {
		t.Fatal(err)
	}
	flowJSON(t, mux, "PUT", "/admin/api/settings/smtp", string(smtpConfiguration), auth, csrf, 200)
	var logs bytes.Buffer
	waitMail := func(kind, state string) {
		deliveryWait(t, func(ctx context.Context) (bool, error) {
			var actual string
			err := db.QueryRowContext(ctx, "SELECT status FROM email_outbox WHERE kind=?", kind).Scan(&actual)
			return actual == state, err
		})
	}
	stop := deliveryWorker(t, h, &logs)
	waitMail(mail.KindOrderConfirmation, "sent")
	stop()
	messages := smtp.snapshot()
	if len(messages) != 1 || !strings.Contains(messages[0].Body, "USD 15.00") || !strings.Contains(messages[0].Body, id) {
		t.Fatal("verified payment confirmation did not reach isolated SMTP")
	}
	shipmentPayload := `{"carrier":"Fixture carrier","tracking_number":"DELIVERY-1","tracking_url":"https://carrier.example.invalid/1"}`
	flowJSON(t, mux, "POST", orderAPI+"/fulfill", shipmentPayload, auth, csrf, 200)
	flowJSON(t, mux, "POST", orderAPI+"/fulfill", shipmentPayload, auth, csrf, 200)
	var history deliveryHistory
	readHistory := func() { flowDecode(t, flowJSON(t, mux, "GET", orderAPI+"/shipment", "", auth, "", 200).Body, &history) }
	readHistory()
	if len(history.Notices) != 1 || history.CurrentVersion != 1 {
		t.Fatal("fulfillment replay duplicated notice")
	}
	originalNotice := history.Notices[0].ID
	originalRevision := history.CurrentRevision
	originalShippedAt := history.ShippedAt
	smtp.dropNextAcknowledgement()
	stop = deliveryWorker(t, h, &logs)
	waitMail(mail.ShipmentKindPrefix+originalNotice, "delivery_unknown")
	stop()
	if len(smtp.snapshot()) != 2 {
		t.Fatal("unknown delivery fixture did not receive first shipment data")
	}

	// Restart the actual worker across many ticks. Unknown DATA cannot be auto-replayed.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	mail.NewWorker(db, h.mailCache, slog.New(slog.NewTextHandler(&logs, nil)), 5*time.Millisecond).Run(ctx)
	cancel()
	if len(smtp.snapshot()) != 2 {
		t.Fatal("worker restart automatically resent an unknown delivery")
	}
	readHistory()
	if history.Notices[0].Status != "delivery_unknown" || history.Notices[0].Resolution != "" {
		t.Fatal("unknown evidence was discarded")
	}
	action := map[string]any{"request_key": uuid.Must(uuid.NewV7()).String(), "action": "received", "confirm_unknown": true}
	actionBody, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	noticeAPI := orderAPI + "/shipment-notices/" + originalNotice + "/actions"
	flowJSON(t, mux, "POST", noticeAPI, string(actionBody), auth, csrf, 200)
	flowJSON(t, mux, "POST", noticeAPI, string(actionBody), auth, csrf, 200)
	readHistory()
	if len(history.Notices) != 1 || history.Notices[0].Resolution != "received" || history.Notices[0].Status != "delivery_unknown" || len(smtp.snapshot()) != 2 {
		t.Fatal("merchant resolution invented SMTP success or resent mail")
	}

	correction, err := json.Marshal(map[string]string{"expected_revision": originalRevision, "carrier": "Corrected carrier", "tracking_number": "DELIVERY-2", "tracking_url": "https://carrier.example.invalid/2"})
	if err != nil {
		t.Fatal(err)
	}
	flowJSON(t, mux, "PATCH", orderAPI+"/shipment", string(correction), auth, csrf, 200)
	readHistory()
	if history.CurrentVersion != 2 || len(history.Notices) != 2 || history.ShippedAt != originalShippedAt {
		t.Fatal("correction lost notice history or original shipped time")
	}
	correctedNotice := history.Notices[0].ID
	stop = deliveryWorker(t, h, &logs)
	waitMail(mail.ShipmentKindPrefix+correctedNotice, "sent")
	stop()
	messages = smtp.snapshot()
	if len(messages) != 3 || !strings.Contains(messages[2].Body, "Tracking version: 2") || !strings.Contains(messages[2].Body, "DELIVERY-2") || !strings.Contains(messages[2].Body, "replaces any earlier") {
		t.Fatal("tracking correction did not reach isolated SMTP")
	}
	for _, message := range messages {
		if strings.Contains(message.Body, grant.Value) {
			t.Fatal("private guest grant leaked into notification")
		}
	}

	// Cumulative partial/full refunds retain shipment records and cannot restore sent stock.
	deliverySignedEvent(t, mux, secret, deliveryRefundEvent("evt_partial_refund", reference, 500), false, 200)
	assertState("fulfilled", 3, 0, 1, 3)
	var orderDetails struct {
		Paid        int    `json:"paid_cents"`
		Refunded    int    `json:"refunded_cents"`
		Fulfillment string `json:"fulfillment_status"`
	}
	readOrder := func() { flowDecode(t, flowJSON(t, mux, "GET", orderAPI, "", auth, "", 200).Body, &orderDetails) }
	readOrder()
	if orderDetails.Paid != 1500 || orderDetails.Refunded != 500 || orderDetails.Fulfillment != "fulfilled" {
		t.Fatal("partial refund lost independent fulfillment/money state")
	}
	deliverySignedEvent(t, mux, secret, deliveryRefundEvent("evt_full_refund", reference, 1500), false, 200)
	deliverySignedEvent(t, mux, secret, deliveryRefundEvent("evt_late_partial_refund", reference, 500), false, 200)
	assertState("refunded", 3, 0, 1, 3)
	readOrder()
	readHistory()
	if orderDetails.Refunded != 1500 || orderDetails.Fulfillment != "fulfilled" || history.CurrentVersion != 2 || history.ShippedAt != originalShippedAt {
		t.Fatal("full or out-of-order refund rewrote shipment history")
	}
	if len(history.Notices) != 2 || history.Notices[0].Status != "sent" || history.Notices[1].ID != originalNotice || history.Notices[1].Status != "delivery_unknown" || history.Notices[1].Resolution != "received" {
		t.Fatal("correction/refund discarded original unknown delivery or merchant resolution")
	}
	if len(smtp.snapshot()) != 3 {
		t.Fatal("refund or replay generated another shipment email")
	}
	for _, private := range []string{secret, "buyer-delivery@example.invalid", grant.Value, "12 Fixture Street"} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("worker logs exposed private data")
		}
	}
}
