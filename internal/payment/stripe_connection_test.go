// Stripe连接验证回归 / Stripe Connection Verification Tests
// 功能：验证官方账户响应、密钥模式、受限密钥及异常/配置变化时不误报成功或泄露凭证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 20:50:12
package payment

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const stripeAccountFixture = `{"id":"acct_connection_fixture","object":"account","charges_enabled":false}`

func TestStripeConnectionUsesAuthenticatedKeyMode(t *testing.T) {
	for _, tc := range []struct {
		name, mode, prefix              string
		env, reject, locked, emptyToken bool
	}{
		{name: "test_secret", mode: "test", prefix: "sk_test_"},
		{name: "default_test_mode", prefix: "sk_test_"},
		{name: "live_secret", mode: "live", prefix: "sk_live_"},
		{name: "test_restricted", mode: "test", prefix: "rk_test_"},
		{name: "live_restricted", mode: "live", prefix: "rk_live_"},
		{name: "environment_override", mode: "live", prefix: "rk_live_", env: true},
		{name: "test_key_in_live", mode: "live", prefix: "sk_test_", reject: true},
		{name: "live_key_in_test", mode: "test", prefix: "rk_live_", reject: true},
		{name: "publishable_is_not_secret", mode: "test", prefix: "pk_test_", reject: true},
		{name: "unknown_mode", mode: "other", prefix: "sk_test_", reject: true},
		{name: "missing_token", mode: "test", prefix: "sk_test_", emptyToken: true, reject: true},
		{name: "malformed_key", mode: "test", prefix: "unknown", reject: true},
		{name: "control_character", mode: "test", prefix: "rk_test_\n", reject: true},
		{name: "locked", mode: "test", locked: true, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secret := tc.prefix + rand.Text()
			if tc.emptyToken {
				secret = tc.prefix
			}
			if tc.locked {
				secret = ""
			}
			keys := &KeyCache{sMode: tc.mode, sSecret: secret}
			if tc.env {
				keys = &KeyCache{stripeEnv: stripeEnv{active: true, mode: tc.mode, secret: secret}, sMode: "test", sSecret: rand.Text()}
			}
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/account" || r.URL.RawQuery != "" || r.ContentLength > 0 {
					t.Error("connection test must only retrieve the authenticated account")
				}
				if r.Header.Get("Authorization") != "Bearer "+secret || r.Header.Get("Stripe-Version") != stripeAPIVersion {
					t.Error("connection test lost authentication or pinned API version")
				}
				_, _ = io.WriteString(w, stripeAccountFixture)
			}))
			defer server.Close()
			provider := NewStripeProvider(keys)
			provider.apiBase = server.URL
			err := provider.TestConnection(context.Background())
			if tc.reject {
				if err == nil || calls.Load() != 0 {
					t.Fatal("invalid credentials or mode must fail before an API request")
				}
				if tc.locked && !errors.Is(err, ErrLocked) {
					t.Fatal("missing credentials must remain locked")
				}
			} else if err != nil || calls.Load() != 1 {
				t.Fatalf("valid account without livemode should pass for %s: %v (requests %d)", tc.name, err, calls.Load())
			}
			if secret != "" && err != nil && strings.Contains(err.Error(), secret) {
				t.Fatal("connection error exposed a credential")
			}
		})
	}
}

func TestStripeConnectionRejectsUnverifiedResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"empty_object", `{}`, 200},
		{"null", `null`, 200},
		{"array", `[]`, 200},
		{"missing_id", `{"object":"account"}`, 200},
		{"missing_object", `{"id":"acct_fixture"}`, 200},
		{"wrong_object", `{"object":"balance","id":"acct_fixture"}`, 200},
		{"invalid_id", `{"object":"account","id":"other_fixture"}`, 200},
		{"empty_account_id", `{"object":"account","id":"acct_"}`, 200},
		{"whitespace_id", `{"object":"account","id":"acct_ fixture"}`, 200},
		{"malformed_json", `{`, 200},
		{"trailing_data", stripeAccountFixture + `{}`, 200},
		{"oversize", stripeAccountFixture + strings.Repeat(" ", 1<<20), 200},
		{"unexpected_success_status", stripeAccountFixture, 201},
		{"unauthorized", `{"error":{"message":"PRIVATE_MARKER"}}`, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secret := "rk_test_" + rand.Text()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, strings.ReplaceAll(tc.body, "PRIVATE_MARKER", secret))
			}))
			defer server.Close()
			provider := NewStripeProvider(&KeyCache{sMode: "test", sSecret: secret})
			provider.apiBase = server.URL
			err := provider.TestConnection(context.Background())
			if err == nil {
				t.Fatal("unverified account response accepted")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("upstream error exposed a credential")
			}
		})
	}
}

func TestStripeConnectionRejectsChangedConfiguration(t *testing.T) {
	for _, action := range []string{"lock", "replace_secret", "replace_mode"} {
		t.Run(action, func(t *testing.T) {
			keys := &KeyCache{sMode: "test", sSecret: "sk_test_" + rand.Text()}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				keys.mu.Lock()
				switch action {
				case "lock":
					keys.sSecret = ""
				case "replace_secret":
					keys.sSecret = "sk_test_" + rand.Text()
				case "replace_mode":
					keys.sMode = "live"
				}
				keys.mu.Unlock()
				_, _ = io.WriteString(w, stripeAccountFixture)
			}))
			defer server.Close()
			provider := NewStripeProvider(keys)
			provider.apiBase = server.URL
			if provider.TestConnection(context.Background()) == nil {
				t.Fatal("stale configuration must not be reported as currently verified")
			}
		})
	}
}

type stripeConnectionBrokenBody struct{ marker string }

func (b stripeConnectionBrokenBody) Read(p []byte) (int, error) {
	return copy(p, stripeAccountFixture), errors.New(b.marker)
}
func (stripeConnectionBrokenBody) Close() error { return nil }

func TestStripeConnectionReadFailureAndCancellation(t *testing.T) {
	secret := "rk_test_" + rand.Text()
	provider := NewStripeProvider(&KeyCache{sMode: "test", sSecret: secret})
	provider.httpClient = &http.Client{Transport: attemptRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: stripeConnectionBrokenBody{marker: secret}}, nil
	})}
	if err := provider.TestConnection(context.Background()); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("read failure must not succeed or expose the upstream error")
	}
	provider.httpClient = &http.Client{Transport: attemptRoundTripper(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.TestConnection(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
