// 凭证配置一致性测试 / Credential Settings Consistency Tests
// 功能：验证收款、SMTP与翻译的拒绝/故障不部分落库，空密码保留且不暴露密钥
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 17:46:55
package admin

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/payment"
)

func TestCredentialSettingsAtomicPersistence(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("PAYPAL_CLIENT_ID", "")
	t.Setenv("SMTP_HOST", "")
	h, mux := newHTTP(t)
	keys := payment.NewKeyCache(h.settings)
	h.svc.SetPaymentKeys(keys)
	sess, csrf := loginAndCookies(t, mux)
	cookies := []*http.Cookie{sess}
	stripeSecret, webhook, paypalSecret, smtpPassword, translationKey := rand.Text(), rand.Text(), rand.Text(), " "+rand.Text()+" ", rand.Text()
	request := func(path string, payload any, want int) {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := doJSON(t, mux, "PUT", "/admin/api/settings/"+path, string(body), cookies, csrf)
		if r.StatusCode != want {
			t.Fatalf("%s save status%d want%d", path, r.StatusCode, want)
		}
		for _, secret := range []string{stripeSecret, webhook, paypalSecret, smtpPassword, translationKey} {
			if strings.Contains(string(r.Body), secret) {
				t.Fatal("response exposed a configuration secret")
			}
		}
	}
	paymentBody := map[string]any{"stripe": map[string]string{"mode": "test", "publishable": "fixture-public", "secret": stripeSecret, "webhook_secret": webhook}, "paypal": map[string]string{"mode": "sandbox", "client_id": "fixture-client", "webhook_id": "fixture-webhook", "secret": paypalSecret}}
	smtpBody := map[string]string{"host": "smtp.example.invalid", "port": "587", "username": "shop@example.invalid", "password": smtpPassword, "from_address": "shop@example.invalid", "from_name": "Fixture shop", "encryption": "starttls"}
	request("payment", paymentBody, 200)
	request("smtp", smtpBody, 200)
	request("translation", map[string]string{"plan": "developer", "api_key": translationKey}, 200)
	type row struct {
		Value     string
		Encrypted int
	}
	snapshot := func() map[string]row {
		t.Helper()
		rows, err := h.svc.db.Query(`SELECT key,value,encrypted FROM setting WHERE key LIKE 'pay.%' OR key LIKE 'smtp.%' OR key LIKE 'translation.%' ORDER BY key`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		out := map[string]row{}
		for rows.Next() {
			var key string
			var value row
			if err := rows.Scan(&key, &value.Value, &value.Encrypted); err != nil {
				t.Fatal(err)
			}
			out[key] = value
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot()
	if len(before) != 17 {
		t.Fatalf("expected17 setting fields got%d", len(before))
	}
	for _, key := range []string{payment.KeyStripeSecret, payment.KeyStripeWebhookSecret, payment.KeyPayPalSecret, "smtp.password", translationKeyKey} {
		if before[key].Encrypted != 1 {
			t.Fatal("secret was not encrypted")
		}
	}
	cacheBefore := keys.Status()
	mailBefore, loaded := h.mailCache.Config()
	if !loaded {
		t.Fatal("saved SMTP not loaded")
	}
	var auditsBefore int
	if err := h.svc.db.QueryRow(`SELECT count(*) FROM audit_event WHERE action IN ('payment.settings_update','smtp.settings_update','translation.settings_update')`).Scan(&auditsBefore); err != nil {
		t.Fatal(err)
	}
	unchanged := func() {
		t.Helper()
		if !reflect.DeepEqual(before, snapshot()) {
			t.Fatal("rejected configuration partially changed storage")
		}
		if keys.Status() != cacheBefore {
			t.Fatal("rejected configuration changed payment cache")
		}
		current, ok := h.mailCache.Config()
		if !ok || current != mailBefore {
			t.Fatal("rejected configuration changed SMTP cache")
		}
		var count int
		if err := h.svc.db.QueryRow(`SELECT count(*) FROM audit_event WHERE action IN ('payment.settings_update','smtp.settings_update','translation.settings_update')`).Scan(&count); err != nil || count != auditsBefore {
			t.Fatal("rejected configuration recorded success")
		}
	}
	request("payment", map[string]any{"stripe": map[string]string{"mode": "live", "publishable": "must-not-save"}, "paypal": map[string]string{"mode": "invalid"}}, 400)
	unchanged()
	if _, err := h.svc.db.Exec(`CREATE TRIGGER reject_credential_write BEFORE UPDATE ON setting WHEN NEW.key IN ('pay.paypal.secret','smtp.password','translation.deepl.api_key') BEGIN SELECT RAISE(ABORT,'injected credential write failure'); END`); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		path    string
		payload any
	}{
		{"payment", map[string]any{"stripe": map[string]string{"mode": "test", "publishable": "must-not-save"}, "paypal": map[string]string{"mode": "sandbox", "client_id": "must-not-save", "secret": rand.Text()}}},
		{"smtp", map[string]string{"host": "changed.example.invalid", "port": "465", "username": "changed@example.invalid", "password": rand.Text(), "from_address": "changed@example.invalid", "encryption": "tls"}},
		{"translation", map[string]string{"plan": "growth", "api_key": rand.Text()}},
	} {
		request(fixture.path, fixture.payload, 500)
		unchanged()
	}
	if _, err := h.svc.db.Exec(`DROP TRIGGER reject_credential_write`); err != nil {
		t.Fatal(err)
	}
	paymentBody["stripe"].(map[string]string)["secret"] = ""
	paymentBody["stripe"].(map[string]string)["webhook_secret"] = ""
	paymentBody["paypal"].(map[string]string)["secret"] = ""
	smtpBody["password"] = ""
	request("payment", paymentBody, 200)
	request("smtp", smtpBody, 200)
	request("translation", map[string]string{"plan": "developer", "api_key": ""}, 200)
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("blank secrets changed stored configuration")
	}
	current, ok := h.mailCache.Config()
	if !ok || current.Password != smtpPassword {
		t.Fatal("SMTP password whitespace not preserved")
	}
	// A read-only second provider cannot leave the first provider partly saved.
	t.Setenv("PAYPAL_CLIENT_ID", "environment-client")
	envKeys := payment.NewKeyCache(h.settings)
	h.svc.SetPaymentKeys(envKeys)
	envBefore := envKeys.Status()
	request("payment", map[string]any{"stripe": map[string]string{"mode": "test", "publishable": "must-not-save"}, "paypal": map[string]string{"mode": "sandbox"}}, 409)
	if !reflect.DeepEqual(before, snapshot()) || envKeys.Status() != envBefore {
		t.Fatal("environment override rejection partially changed payment configuration")
	}
}
