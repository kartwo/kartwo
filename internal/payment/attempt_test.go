// 支付尝试回归测试 / Payment Attempt Regression Tests
// 功能：网络响应丢失、参数冻结、服务重启、并发认领及配置变更保护
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 13:18:17
package payment

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"
)

func checkoutInput(ref string) OrderForPayment {
	return OrderForPayment{PublicID: ref, Currency: "USD", AmountCents: 1, Email: "untrusted@example.com", Description: "Order", SuccessURL: "https://shop.example/success", CancelURL: "https://shop.example/cancel"}
}

func TestCheckoutAttemptFreezesRequestAndSurvivesRestart(t *testing.T) {
	svc, db, kek := newTestSvc(t)
	seedOrder(t, db, "attempt-retry", 2500, "USD")
	var calls atomic.Int64
	var firstBody, firstKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		key := r.Header.Get("Idempotency-Key")
		if calls.Add(1) == 1 {
			firstBody, firstKey = string(body), key
			// 模拟网关已接收请求，但第一次调用未拿到可用响应。
			http.Error(w, "temporary gateway response failure", http.StatusBadGateway)
			return
		}
		if key == "" || key != firstKey || string(body) != firstBody {
			t.Errorf("retry changed key/body")
		}
		_, _ = w.Write([]byte(`{"id":"cs_retry","url":"https://checkout.example/retry"}`))
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	input := checkoutInput("attempt-retry")
	if _, err := svc.StartCheckout(context.Background(), "stripe", input); err == nil {
		t.Fatal("first response should fail")
	}

	// 真正关闭并重新打开文件数据库，同时重新解锁新的密钥缓存。
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
	restarted := NewService(reopened, cfg, keys)
	restarted.stripe.apiBase = ts.URL
	input.Description, input.SuccessURL, input.AmountCents = "changed", "https://other.example/success", 999999
	got, err := restarted.StartCheckout(context.Background(), "stripe", input)
	if err != nil || got != "https://checkout.example/retry" {
		t.Fatalf("restart retry: %q %v", got, err)
	}
	values, err := url.ParseQuery(firstBody)
	if err != nil {
		t.Fatal(err)
	}
	if values.Get("line_items[0][price_data][unit_amount]") != "2500" || values.Get("customer_email") != "a@b.com" {
		t.Fatalf("must use authoritative amount/email: %s", firstBody)
	}
	if firstKey == "" || len(firstKey) != 36 {
		t.Fatalf("key=%q", firstKey)
	}
	for i := 0; i < 3; i++ {
		if _, err := restarted.StartCheckout(context.Background(), "stripe", input); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("ready session should be reused, requests=%d", calls.Load())
	}
	var count int
	if err := reopened.QueryRow(`SELECT COUNT(*) FROM payment_attempt WHERE state='ready' AND provider_reference='cs_retry'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("attempt count=%d err=%v", count, err)
	}
	if applied, err := migrate.Run(context.Background(), reopened, migrations.FS); err != nil || applied != 0 {
		t.Fatalf("migration reentry: %d %v", applied, err)
	}
}

func TestCheckoutAttemptConcurrentClaim(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "attempt-concurrent", 2500, "USD")
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		_, _ = w.Write([]byte(`{"id":"cs_concurrent","url":"https://checkout.example/concurrent"}`))
	}))
	defer ts.Close()
	defer close(release)
	svc.stripe.apiBase = ts.URL
	result := make(chan error, 1)
	go func() {
		_, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("attempt-concurrent"))
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("provider not called")
	}
	const requests = 12
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			independent := NewService(db, svc.settings, svc.keys)
			independent.stripe.apiBase = ts.URL
			_, err := independent.StartCheckout(context.Background(), "stripe", checkoutInput("attempt-concurrent"))
			if !errors.Is(err, ErrPaymentInProgress) {
				t.Errorf("concurrent call: %v", err)
			}
		}()
	}
	wg.Wait()
	// 解除 handler 阻塞；defer 仍负责最终关闭通道。
	release <- struct{}{}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("requests=%d", calls.Load())
	}
}

func TestCheckoutAttemptLeaseRecoveryAndConfigChange(t *testing.T) {
	svc, db, kek := newTestSvc(t)
	seedOrder(t, db, "attempt-crash", 2500, "USD")
	ctx := context.Background()
	claimed, err := svc.claimCheckout(ctx, "stripe", checkoutInput("attempt-crash"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartCheckout(ctx, "stripe", checkoutInput("attempt-crash")); !errors.Is(err, ErrPaymentInProgress) {
		t.Fatalf("lease: %v", err)
	}
	if _, err := db.Exec(`UPDATE payment_attempt SET lease_until=0 WHERE public_id=?`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != claimed.ID {
			t.Error("lease recovery must keep key")
		}
		_, _ = w.Write([]byte(`{"id":"cs_crash","url":"https://checkout.example/crash"}`))
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	if _, err := svc.StartCheckout(ctx, "stripe", checkoutInput("attempt-crash")); err != nil {
		t.Fatal(err)
	}
	if err := svc.settings.SetEncrypted(ctx, KeyStripeSecret, []byte("sk_test_changed"), kek); err != nil {
		t.Fatal(err)
	}
	if err := svc.keys.Unlock(ctx, kek); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartCheckout(ctx, "stripe", checkoutInput("attempt-crash")); !errors.Is(err, ErrPaymentConfigChanged) {
		t.Fatalf("config change: %v", err)
	}
}

func TestPayPalAttemptAndCaptureIdempotencyHeaders(t *testing.T) {
	var createKey, captureKey string
	var creates, captures int
	svc, db, _ := newPayPalSvc(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case r.URL.Path == "/v2/checkout/orders":
			creates++
			createKey = r.Header.Get("PayPal-Request-Id")
			_, _ = w.Write([]byte(`{"id":"PP1","links":[{"rel":"approve","href":"https://paypal.example/approve"}]}`))
		case strings.HasSuffix(r.URL.Path, "/capture"):
			captures++
			key := r.Header.Get("PayPal-Request-Id")
			if captureKey != "" && captureKey != key {
				t.Error("capture key changed")
			}
			captureKey = key
			_, _ = w.Write([]byte(`{"status":"COMPLETED","purchase_units":[{"payments":{"captures":[{"id":"CAP1","status":"COMPLETED","custom_id":"attempt-paypal","amount":{"currency_code":"USD","value":"25.00"}}]}}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	seedOrder(t, db, "attempt-paypal", 2500, "USD")
	for i := 0; i < 2; i++ {
		if _, err := svc.StartCheckout(context.Background(), "paypal", checkoutInput("attempt-paypal")); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.CapturePayPal(context.Background(), "PP1"); err != nil {
			t.Fatal(err)
		}
	}
	if creates != 1 || captures != 2 || len(createKey) != 36 || captureKey == "" || createKey == captureKey {
		t.Fatalf("creates=%d captures=%d create=%q capture=%q", creates, captures, createKey, captureKey)
	}
}

func TestCheckoutAttemptDoesNotReuseAfterOrderClosed(t *testing.T) {
	for _, status := range []string{"paid", "expired"} {
		t.Run(status, func(t *testing.T) {
			svc, db, _ := newTestSvc(t)
			seedOrder(t, db, "attempt-closed", 2500, "USD")
			var calls atomic.Int64
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"id":"cs_closed","url":"https://checkout.example/closed"}`))
			}))
			defer ts.Close()
			svc.stripe.apiBase = ts.URL
			if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("attempt-closed")); err != nil {
				t.Fatal(err)
			}
			var err error
			if status == "paid" {
				_, err = db.Exec(`UPDATE "order" SET status='paid' WHERE public_id='attempt-closed'`)
			} else {
				_, err = db.Exec(`UPDATE "order" SET reservation_expires_at='2000-01-01T00:00:00.000Z' WHERE public_id='attempt-closed'`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("attempt-closed")); !errors.Is(err, ErrOrderExpired) {
				t.Fatalf("closed order: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("closed order called provider %d times", calls.Load())
			}
		})
	}
}

func TestCheckoutAttemptSavesSuccessAfterClientCancellation(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "attempt-disconnect", 2500, "USD")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.stripe.httpClient = &http.Client{Transport: attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_disconnect","url":"https://checkout.example/disconnect"}`)), Header: make(http.Header)}, nil
	})}
	if _, err := svc.StartCheckout(ctx, "stripe", checkoutInput("attempt-disconnect")); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM payment_attempt`).Scan(&state); err != nil || state != "ready" {
		t.Fatalf("state=%q err=%v", state, err)
	}
}

type attemptRoundTripper func(*http.Request) (*http.Response, error)

func (f attemptRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckoutAttemptRejectsInvalidGatewayURL(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "attempt-url", 2500, "USD")
	svc.stripe.httpClient = &http.Client{Transport: attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_bad","url":"javascript:alert(1)"}`)), Header: make(http.Header)}, nil
	})}
	if _, err := svc.StartCheckout(context.Background(), "stripe", checkoutInput("attempt-url")); err == nil {
		t.Fatal("must reject invalid redirect")
	}
	var state, link, lease string
	if err := db.QueryRow(`SELECT state,redirect_url,lease_token FROM payment_attempt`).Scan(&state, &link, &lease); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || link != "" || lease != "" {
		t.Fatalf("state=%s link=%s lease=%s", state, link, lease)
	}
}

func TestCheckoutAttemptRecoversRejectedExpiryWithSameKey(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedOrder(t, db, "attempt-expiry-recovery", 2500, "USD")
	ctx := context.Background()
	claim, err := svc.claimCheckout(ctx, "stripe", checkoutInput("attempt-expiry-recovery"))
	if err != nil {
		t.Fatal(err)
	}
	// 模拟进程在发出请求前崩溃，重启时租约和旧 expires_at 已经过时。
	if _, err := db.Exec(`UPDATE payment_attempt SET lease_until=0,request_json=json_set(request_json,'$.ExpiresAt',1)`); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Idempotency-Key") != claim.ID {
			t.Error("expiry recovery rotated key")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.PostForm.Get("expires_at") == "1" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","param":"expires_at","message":"too soon"}}`))
			return
		}
		if r.PostForm.Get("success_url") != claim.Request.SuccessURL || r.PostForm.Get("line_items[0][price_data][unit_amount]") != "2500" {
			t.Error("expiry recovery changed other parameters")
		}
		_, _ = w.Write([]byte(`{"id":"cs_expiry_recovery","url":"https://checkout.example/recovered"}`))
	}))
	defer ts.Close()
	svc.stripe.apiBase = ts.URL
	if _, err := svc.StartCheckout(ctx, "stripe", checkoutInput("attempt-expiry-recovery")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests=%d", calls.Load())
	}
	var expiry int64
	if err := db.QueryRow(`SELECT json_extract(request_json,'$.ExpiresAt') FROM payment_attempt`).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	if expiry <= time.Now().Add(30*time.Minute).Unix() {
		t.Fatalf("expiry not persisted: %d", expiry)
	}
}
