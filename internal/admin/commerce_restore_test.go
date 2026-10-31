// 售后整店恢复回归 / After Sale Store Restore Flow
// 功能：实际导出恢复、旧授权失效、原账号解密配置、重新验证订单及审核原键不重写
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 23:56:04
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store"
	"github.com/kartwo/kartwo/internal/storefront"
	"github.com/kartwo/kartwo/migrations"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

func TestAfterSaleStoreRestoreRequiresFreshAccessAndPreservesOperation(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	ctx := context.Background()
	h, _ := newHTTP(t)
	source := http.NewServeMux()
	h.Register(source)
	customer := storefront.NewHTTP(storefront.New(h.svc.db), cart.New(h.svc.db), h.orders, h.settings, nil, redirect.New(h.svc.db), "Restore fixture", "https://store.example", nil)
	customer.ConfigureGuestMail(h.mailCache)
	customer.Register(source)
	owner, csrf := loginAndCookies(t, source)
	auth := []*http.Cookie{owner}
	id, guestToken, c := afterSaleAPIOrder(t, h)
	path := "/admin/api/orders/" + id
	guestPath := "/order/" + id + "/after-sales"
	oldGuest := &http.Cookie{Name: "kartwo_order_access", Value: guestToken}
	message := "Restore fixture: approved; actual refund remains a separate explicit operation."
	input, _ := json.Marshal(map[string]any{"request_key": uuid.Must(uuid.NewV7()).String(), "expected_version": 1, "action": "approve", "message": message})
	actionPath := path + "/after-sales/" + c.PublicID + "/actions"
	flowJSON(t, source, "POST", actionPath, string(input), auth, csrf, 200)
	var before struct {
		Snapshot order.AfterSaleSnapshot `json:"snapshot"`
	}
	flowDecode(t, flowJSON(t, source, "GET", path+"/after-sales", "", auth, "", 200).Body, &before)
	if len(before.Snapshot.Cases) != 1 || before.Snapshot.Cases[0].State != "approved" {
		t.Fatal("source review missing")
	}
	if response := flowRequest(t, source, "GET", guestPath, "", "", oldGuest); response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(message)) {
		t.Fatal("source private sale not readable")
	}
	// Simulate passage of the one-minute resend interval in the isolated fixture;
	// keep expiry, hashes and normal service-issued authorization unchanged.
	if _, err := h.svc.db.Exec(`UPDATE order_guest_challenge SET requested_at=requested_at-61`); err != nil {
		t.Fatal(err)
	}
	pendingBrowser, _ := order.NewGuestToken()
	if err := h.orders.RequestGuestAccess(ctx, id, "after-sale-buyer@example.invalid", pendingBrowser); err != nil {
		t.Fatal(err)
	}
	var pendingBody string
	if err := h.svc.db.QueryRow(`SELECT body FROM email_outbox WHERE kind GLOB 'guest_access:*' ORDER BY id DESC LIMIT 1`).Scan(&pendingBody); err != nil {
		t.Fatal(err)
	}
	pendingCode := regexp.MustCompile(`code is: ([0-9]{8})`).FindStringSubmatch(pendingBody)
	if len(pendingCode) != 2 {
		t.Fatal("pending source challenge missing")
	}
	if _, err := h.svc.db.Exec(`UPDATE order_guest_challenge SET requested_at=requested_at-61`); err != nil {
		t.Fatal(err)
	}
	var role struct {
		ID string `json:"public_id"`
	}
	flowDecode(t, flowJSON(t, source, "POST", "/admin/api/roles", `{"name":"Restore reviewer","permissions":["orders.view","orders.customer_data","orders.aftersales"]}`, auth, csrf, 201).Body, &role)
	var invite struct {
		Path string `json:"invite_path"`
	}
	flowDecode(t, flowJSON(t, source, "POST", "/admin/api/users", `{"username":"restore-pending-reviewer","role_id":"`+role.ID+`"}`, auth, csrf, 201).Body, &invite)
	link, err := url.Parse(invite.Path)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.Parse(link.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	inviteToken := fragment.Query().Get("token")
	if inviteToken == "" {
		t.Fatal("source invitation missing")
	}
	smtpPassword, _ := order.NewGuestToken()
	smtpInput, _ := json.Marshal(map[string]string{"host": "smtp.restore.example.invalid", "port": "587", "username": "restore-fixture", "password": smtpPassword, "from_address": "restore@example.invalid", "encryption": "starttls"})
	flowJSON(t, source, "PUT", "/admin/api/settings/smtp", string(smtpInput), auth, csrf, 200)
	var encrypted int
	var ciphertext string
	if err = h.svc.db.QueryRow(`SELECT encrypted,value FROM setting WHERE key=?`, mail.KeySMTPPassword).Scan(&encrypted, &ciphertext); err != nil || encrypted != 1 || ciphertext == smtpPassword {
		t.Fatal("source SMTP secret is not encrypted")
	}
	mediaPath := filepath.Join(h.exporter.DataDir(), "media", "originals", "restore-fixture.txt")
	if err = os.MkdirAll(filepath.Dir(mediaPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(mediaPath, []byte("virtual-media-original"), 0600); err != nil {
		t.Fatal(err)
	}
	exported := flowJSON(t, source, "GET", "/admin/api/export", "", auth, "", 200)
	archive := filepath.Join(t.TempDir(), "store.zip")
	if err = os.WriteFile(archive, exported.Body, 0600); err != nil {
		t.Fatal(err)
	}
	// The backup predates a genuine revocation/logout. A historical file cannot
	// know these later facts; fresh authorization after restore is required.
	flowJSON(t, source, "POST", path+"/revoke-access", `{}`, auth, csrf, 200)
	flowJSON(t, source, "POST", "/admin/api/logout", `{}`, auth, csrf, 200)
	restoredDir := filepath.Join(t.TempDir(), "restored-store")
	if _, err = backup.Restore(archive, restoredDir); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(&config.Config{DataDir: restoredDir, DBPath: filepath.Join(restoredDir, "shop.db"), DBEngine: "sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if n, e := migrate.Run(ctx, st.DB, migrations.FS); e != nil || n != 0 {
		t.Fatalf("restored migrations %d %v", n, e)
	}
	set := settings.New(st.DB)
	svc := New(st.DB)
	ord := order.New(st.DB, set)
	cache := mail.NewCache(set)
	svc.SetMailKeys(cache)
	restoredHTTP := NewHTTP(svc, catalog.New(st.DB), nil, nil, set, ord, nil, cache, backup.New(st.DB, restoredDir, "restored-fixture"), BackupConfig{}, "", false, nil)
	restored := http.NewServeMux()
	restoredHTTP.Register(restored)
	restoredCustomer := storefront.NewHTTP(storefront.New(st.DB), cart.New(st.DB), ord, set, nil, redirect.New(st.DB), "Restored fixture", "https://store.example", nil)
	restoredCustomer.ConfigureGuestMail(cache)
	restoredCustomer.Register(restored)
	flowJSON(t, restored, "GET", path+"/after-sales", "", auth, "", 401)
	view := flowRequest(t, restored, "GET", guestPath, "", "", oldGuest)
	if view.Code != 200 || bytes.Contains(view.Body.Bytes(), []byte(message)) || bytes.Contains(view.Body.Bytes(), []byte("PRIVATE-AFTERSALE-REASON")) {
		t.Fatal("restore resurrected revoked private guest access")
	}
	if _, err = ord.VerifyGuestAccess(ctx, id, pendingBrowser, pendingCode[1]); !errors.Is(err, order.ErrGuestVerification) {
		t.Fatal("restore revived an old pending email code", err)
	}
	acceptance, _ := json.Marshal(map[string]string{"token": inviteToken, "password": "isolated-restore-invite-password"})
	flowJSON(t, restored, "POST", "/admin/api/invites/accept", string(acceptance), nil, "", 400)
	signedIn := flowJSON(t, restored, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "", 200)
	newOwner := []*http.Cookie{flowCookie(t, signedIn.Cookies, sessionCookie)}
	newCSRF := flowCookie(t, signedIn.Cookies, csrfCookie).Value
	cfg, ready := cache.Config()
	if !ready || cfg.Password != smtpPassword {
		t.Fatal("original owner login failed to decrypt restored settings")
	}
	var after struct {
		Snapshot order.AfterSaleSnapshot `json:"snapshot"`
	}
	flowDecode(t, flowJSON(t, restored, "GET", path+"/after-sales", "", newOwner, "", 200).Body, &after)
	if !reflect.DeepEqual(before.Snapshot, after.Snapshot) {
		t.Fatal("restore changed original sale facts, policy or operation history")
	}
	for range 2 {
		flowJSON(t, restored, "POST", actionPath, string(input), newOwner, newCSRF, 200)
	}
	var events, audits, refunds int
	if err = st.DB.QueryRow(`SELECT (SELECT count(*) FROM after_sale_event),(SELECT count(*) FROM audit_event WHERE action='order.after_sale.approve'),(SELECT count(*) FROM refund_request)`).Scan(&events, &audits, &refunds); err != nil || events != 2 || audits != 1 || refunds != 0 {
		t.Fatal("business replay after restore duplicated review or moved funds", err)
	}
	freshBrowser, _ := order.NewGuestToken()
	if err = ord.RequestGuestAccess(ctx, id, "after-sale-buyer@example.invalid", freshBrowser); err != nil {
		t.Fatal(err)
	}
	var freshBody string
	if err = st.DB.QueryRow(`SELECT body FROM email_outbox WHERE kind GLOB 'guest_access:*' ORDER BY id DESC LIMIT 1`).Scan(&freshBody); err != nil {
		t.Fatal(err)
	}
	freshCode := regexp.MustCompile(`code is: ([0-9]{8})`).FindStringSubmatch(freshBody)
	if len(freshCode) != 2 {
		t.Fatal("fresh guest email not queued")
	}
	freshToken, err := ord.VerifyGuestAccess(ctx, id, freshBrowser, freshCode[1])
	if err != nil {
		t.Fatal(err)
	}
	view = flowRequest(t, restored, "GET", guestPath, "", "", &http.Cookie{Name: "kartwo_order_access", Value: freshToken})
	if view.Code != 200 || !bytes.Contains(view.Body.Bytes(), []byte(message)) {
		t.Fatal("fresh verified guest could not resume original sale")
	}
	var leftover int
	if err = st.DB.QueryRow(`SELECT count(*) FROM email_outbox WHERE kind GLOB 'guest_access:*' AND id<=2 AND body<>''`).Scan(&leftover); err != nil || leftover != 0 {
		t.Fatal("restored expired code plaintext retained", err)
	}
	if contents, e := os.ReadFile(filepath.Join(restoredDir, "media", "originals", "restore-fixture.txt")); e != nil || string(contents) != "virtual-media-original" {
		t.Fatal("restored media differs", e)
	}
}
