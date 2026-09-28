// 后台客服留言测试 / Admin Contact Tests
// 功能：验证店主配置、收件箱鉴权、公开演示敏感信息隔离
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 15:30:00
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/contact"
)

func TestContactAdminOwnerAndDemoIsolation(t *testing.T) {
	h, mux := newHTTP(t)
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: 45 * time.Minute, MaxProducts: 3, MaxImageBytes: 2 << 20, CleanupInterval: 5 * time.Minute})
	sess, csrf := loginAndCookies(t, mux)
	owner := []*http.Cookie{sess}
	settingsBody := `{"enabled":true,"retention_months":12,"ip_retention_days":90}`
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/contact", settingsBody, owner, ""); got.StatusCode != http.StatusForbidden {
		t.Fatalf("CSRF=%d", got.StatusCode)
	}
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/contact", settingsBody, owner, csrf); got.StatusCode != http.StatusOK {
		t.Fatalf("owner config=%d %s", got.StatusCode, got.Body)
	}
	id, err := h.contact.Submit(context.Background(), contact.Input{Name: "Secret Name", Email: "private@example.com", Topic: "order", Body: "Please find my order", IP: "203.0.113.3"})
	if err != nil {
		t.Fatal(err)
	}
	if got := doJSON(t, mux, "GET", "/admin/api/contact/messages", "", nil, ""); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", got.StatusCode)
	}
	if got := doJSON(t, mux, "GET", "/admin/api/contact/messages", "", owner, ""); got.StatusCode != http.StatusOK || !bytes.Contains(got.Body, []byte("private@example.com")) {
		t.Fatalf("owner list=%d %s", got.StatusCode, got.Body)
	}
	demoLogin := doJSON(t, mux, "POST", "/admin/api/demo-session", "", nil, "")
	if demoLogin.StatusCode != http.StatusCreated {
		t.Fatalf("demo login=%d", demoLogin.StatusCode)
	}
	var demoSess *http.Cookie
	var demoCSRF string
	for _, cookie := range demoLogin.Cookies {
		if cookie.Name == sessionCookie {
			demoSess = cookie
		}
		if cookie.Name == csrfCookie {
			demoCSRF = cookie.Value
		}
	}
	demo := []*http.Cookie{demoSess}
	list := doJSON(t, mux, "GET", "/admin/api/contact/messages", "", demo, "")
	if list.StatusCode != http.StatusOK || !bytes.Contains(list.Body, []byte(`"demo_hidden":true`)) || bytes.Contains(list.Body, []byte("private@example.com")) {
		t.Fatalf("demo list=%d %s", list.StatusCode, list.Body)
	}
	for _, path := range []string{"/admin/api/contact/messages/" + id, "/admin/api/contact/messages/unknown"} {
		if got := doJSON(t, mux, "GET", path, "", demo, ""); got.StatusCode != http.StatusForbidden {
			t.Fatalf("demo detail=%d", got.StatusCode)
		}
	}
	if got := doJSON(t, mux, "PATCH", "/admin/api/contact/messages/"+id, `{"status":"closed"}`, demo, demoCSRF); got.StatusCode != http.StatusForbidden {
		t.Fatalf("demo write=%d", got.StatusCode)
	}
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/contact", settingsBody, demo, demoCSRF); got.StatusCode != http.StatusForbidden {
		t.Fatalf("demo config=%d", got.StatusCode)
	}
	if forged := doJSON(t, mux, "PATCH", "/admin/api/contact/messages/"+id, `{"status":"processing","processing_by_name":"forged"}`, owner, csrf); forged.StatusCode != http.StatusBadRequest {
		t.Fatalf("actor spoof should be rejected: %d %s", forged.StatusCode, forged.Body)
	}
	processing := doJSON(t, mux, "PATCH", "/admin/api/contact/messages/"+id, `{"status":"processing"}`, owner, csrf)
	var processingMessage contact.Message
	if processing.StatusCode != http.StatusOK || json.Unmarshal(processing.Body, &processingMessage) != nil || processingMessage.ProcessingByName != "admin" || processingMessage.ProcessingByID == "" {
		t.Fatalf("owner processing=%d %s", processing.StatusCode, processing.Body)
	}
	updated := doJSON(t, mux, "PATCH", "/admin/api/contact/messages/"+id, `{"status":"closed"}`, owner, csrf)
	var m contact.Message
	if updated.StatusCode != http.StatusOK || json.Unmarshal(updated.Body, &m) != nil || m.Status != "closed" || m.ClosedByName != "admin" || m.ClosedByID == "" {
		t.Fatalf("owner close=%d %s", updated.StatusCode, updated.Body)
	}
	if again := doJSON(t, mux, "PATCH", "/admin/api/contact/messages/"+id, `{"status":"closed"}`, owner, csrf); again.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate closure=%d %s", again.StatusCode, again.Body)
	}
}
