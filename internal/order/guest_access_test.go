// 游客订单授权测试 / Guest Order Authorization Tests
// 功能：验证订单隔离、验证码配额/尝试/到期/重放、并发兑换、原子下单及迁移重入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package order

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"
)

func guestFixture(t *testing.T) (*sql.DB, *Service) {
	t.Helper()
	db, s, _, _ := setup(t)
	for _, query := range []string{
		`INSERT INTO customer(id,public_id,email,name) VALUES(501,'guest-customer','buyer@example.com','Private buyer')`,
		`INSERT INTO "order"(public_id,customer_id,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES('guest-order',501,'buyer@example.com','Private buyer','Private address',100,100)`,
		`INSERT INTO "order"(public_id,customer_id,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES('other-order',501,'buyer@example.com','Private buyer','Other address',200,200)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return db, s
}
func guestBrowser(t *testing.T) string {
	t.Helper()
	v, err := NewGuestToken()
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func latestCode(t *testing.T, db *sql.DB) string {
	t.Helper()
	var body string
	if err := db.QueryRow(`SELECT body FROM email_outbox WHERE kind GLOB 'guest_access:*' ORDER BY id DESC LIMIT 1`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`code is: ([0-9]{8})`).FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("missing verification code")
	}
	return m[1]
}
func requestCode(t *testing.T, s *Service, db *sql.DB, browser string) string {
	t.Helper()
	if err := s.RequestGuestAccess(context.Background(), "guest-order", "BUYER@example.com", browser); err != nil {
		t.Fatal(err)
	}
	return latestCode(t, db)
}

func TestGuestAccessScopeReplayAndRestart(t *testing.T) {
	db, s := guestFixture(t)
	ctx := context.Background()
	browser := guestBrowser(t)
	if ok, err := s.GuestAuthorized(ctx, "guest-order", browser); err != nil || ok {
		t.Fatal("challenge is not an access credential")
	}
	code := requestCode(t, s, db, browser)
	for _, tc := range []struct{ id, browser string }{{"other-order", browser}, {"guest-order", guestBrowser(t)}} {
		if _, err := s.VerifyGuestAccess(ctx, tc.id, tc.browser, code); !errors.Is(err, ErrGuestVerification) {
			t.Fatalf("cross-scope verify: %v", err)
		}
	}
	token, err := s.VerifyGuestAccess(ctx, "guest-order", browser, code)
	if err != nil {
		t.Fatal(err)
	}
	if token == browser {
		t.Fatal("verification must rotate to a new access token")
	}
	restarted := New(db, settings.New(db))
	if ok, err := restarted.GuestAuthorized(ctx, "guest-order", token); err != nil || !ok {
		t.Fatal("grant must survive a service restart")
	}
	if ok, _ := restarted.GuestAuthorized(ctx, "other-order", token); ok {
		t.Fatal("cross-order grant")
	}
	if _, err := s.VerifyGuestAccess(ctx, "guest-order", browser, code); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("code replay accepted")
	}
	var hash, body string
	if err := db.QueryRow(`SELECT token_hash FROM order_guest_access`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == token || hash == browser || len(hash) != 64 {
		t.Fatal("plaintext or invalid token digest stored")
	}
	if err := db.QueryRow(`SELECT body FROM email_outbox WHERE kind GLOB 'guest_access:*'`).Scan(&body); err != nil || body != "" {
		t.Fatal("consumed code left in outbox")
	}
	raw, err := fs.ReadFile(migrations.FS, "0034_guest_order_access.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GuestAuthorized(ctx, "guest-order", token); !ok {
		t.Fatal("reentrant migration lost grant")
	}
	if err = s.RevokeGuestAccess(ctx, "other-order", token); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GuestAuthorized(ctx, "guest-order", token); !ok {
		t.Fatal("wrong-order logout revoked access")
	}
	if err = s.RevokeGuestAccess(ctx, "guest-order", token); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GuestAuthorized(ctx, "guest-order", token); ok {
		t.Fatal("logout left grant active")
	}
}
func TestGuestChallengeLimitsExpiryAndConcurrentConsumption(t *testing.T) {
	t.Run("attempt limit", func(t *testing.T) {
		db, s := guestFixture(t)
		b := guestBrowser(t)
		code := requestCode(t, s, db, b)
		for i := 0; i < 5; i++ {
			if _, err := s.VerifyGuestAccess(context.Background(), "guest-order", b, "wrong"); !errors.Is(err, ErrGuestVerification) {
				t.Fatal(err)
			}
		}
		if _, err := s.VerifyGuestAccess(context.Background(), "guest-order", b, code); !errors.Is(err, ErrGuestVerification) {
			t.Fatal("exhausted code accepted")
		}
		var attempts int
		var body string
		if err := db.QueryRow(`SELECT attempts FROM order_guest_challenge`).Scan(&attempts); err != nil || attempts != 5 {
			t.Fatalf("attempts=%d err=%v", attempts, err)
		}
		if err := db.QueryRow(`SELECT body FROM email_outbox`).Scan(&body); err != nil || body != "" {
			t.Fatal("locked code body retained")
		}
	})
	t.Run("expired", func(t *testing.T) {
		db, s := guestFixture(t)
		b := guestBrowser(t)
		code := requestCode(t, s, db, b)
		if _, err := db.Exec(`UPDATE order_guest_challenge SET expires_at=?`, time.Now().Unix()-1); err != nil {
			t.Fatal(err)
		}
		if _, err := s.VerifyGuestAccess(context.Background(), "guest-order", b, code); !errors.Is(err, ErrGuestVerification) {
			t.Fatal("expired code accepted")
		}
	})
	t.Run("concurrent", func(t *testing.T) {
		db, s := guestFixture(t)
		b := guestBrowser(t)
		code := requestCode(t, s, db, b)
		var wins atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.VerifyGuestAccess(context.Background(), "guest-order", b, code); err == nil {
					wins.Add(1)
				} else if !errors.Is(err, ErrGuestVerification) {
					t.Errorf("verify: %v", err)
				}
			}()
		}
		wg.Wait()
		if wins.Load() != 1 {
			t.Fatalf("successful exchanges=%d", wins.Load())
		}
	})
}
func TestGuestChallengeSendQuotaAndNoEnumerationWrites(t *testing.T) {
	db, s := guestFixture(t)
	ctx := context.Background()
	b := guestBrowser(t)
	for _, tc := range []struct{ id, email string }{{"missing", "buyer@example.com"}, {"guest-order", "wrong@example.com"}} {
		if err := s.RequestGuestAccess(ctx, tc.id, tc.email, b); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unknown lookup enqueued email")
	}
	requestCode(t, s, db, b)
	for i := 0; i < 5; i++ {
		if err := s.RequestGuestAccess(ctx, "guest-order", "buyer@example.com", guestBrowser(t)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_guest_challenge`).Scan(&count); err != nil || count != 1 {
		t.Fatal("cooldown bypass")
	}
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`UPDATE order_guest_challenge SET requested_at=requested_at-61`); err != nil {
			t.Fatal(err)
		}
		if err := s.RequestGuestAccess(ctx, "guest-order", "buyer@example.com", b); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_guest_challenge`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("hourly count=%d err=%v", count, err)
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_guest_challenge WHERE consumed_at=0`).Scan(&active); err != nil || active != 1 {
		t.Fatal("resend left older codes active")
	}
}
func TestGuestCheckoutGrantAtomicity(t *testing.T) {
	db, s, crt, cat := setup(t)
	ctx := context.Background()
	v := seedVariant(t, cat, "guest-tee", 1200, 3)
	cartID, _, err := crt.GetOrCreate(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = crt.AddItem(ctx, cartID, v, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER deny_guest_access BEFORE INSERT ON order_guest_access BEGIN SELECT RAISE(FAIL,'test grant failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.CheckoutForGuest(ctx, cartID, info()); err == nil {
		t.Fatal("grant failure must fail checkout")
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM "order"`).Scan(&count); err != nil || count != 0 {
		t.Fatal("orphan order after grant failure")
	}
	if _, reserved := reservedOf(t, db, "guest-tee"); reserved != 0 {
		t.Fatal("stock not rolled back")
	}
	if _, err = db.Exec(`DROP TRIGGER deny_guest_access`); err != nil {
		t.Fatal(err)
	}
	id, token, err := s.CheckoutForGuest(ctx, cartID, info())
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.GuestAuthorized(ctx, id, token); err != nil || !ok {
		t.Fatal("checkout grant missing")
	}
	if _, _, err = s.CheckoutForGuest(ctx, cartID, info()); !errors.Is(err, ErrCartConverted) {
		t.Fatal("duplicate checkout grant")
	}
	if _, err = db.Exec(`UPDATE order_guest_access SET expires_at=?`, time.Now().Unix()-1); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GuestAuthorized(ctx, id, token); ok {
		t.Fatal("expired grant accepted")
	}
	for _, bad := range []string{"", strings.Repeat("a", 43), token + "x", "invalid"} {
		if ok, _ := s.GuestAuthorized(ctx, id, bad); ok {
			t.Fatal("malformed grant accepted")
		}
	}
}

func TestGuestRevocationAndRetention(t *testing.T) {
	db, s := guestFixture(t)
	ctx := context.Background()
	b := guestBrowser(t)
	code := requestCode(t, s, db, b)
	token, err := s.VerifyGuestAccess(ctx, "guest-order", b, code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE order_guest_challenge SET requested_at=requested_at-61`); err != nil {
		t.Fatal(err)
	}
	nextCode := requestCode(t, s, db, b)
	if err = s.RevokeAllGuestAccess(ctx, "guest-order"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GuestAuthorized(ctx, "guest-order", token); ok {
		t.Fatal("revoked token accepted")
	}
	if _, err = s.VerifyGuestAccess(ctx, "guest-order", b, nextCode); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("revoked challenge accepted")
	}
	var count int
	if err = s.CleanGuestAccess(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM order_guest_challenge`).Scan(&count); err != nil || count != 2 {
		t.Fatal("cleanup erased current quota evidence")
	}
	if err = s.CleanGuestAccess(ctx, time.Now().Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM order_guest_challenge`).Scan(&count); err != nil || count != 0 {
		t.Fatal("old challenge evidence retained")
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM email_outbox WHERE body<>''`).Scan(&count); err != nil || count != 0 {
		t.Fatal("purged challenge left a code in outbox")
	}
}

func TestGuestRecoveryRejectsMalformedLegacyEmail(t *testing.T) {
	db, s := guestFixture(t)
	for _, email := range []string{"Buyer <buyer@example.com>", "buyer@example.com\r\nBcc: stranger@example.com", "buyer@example.com,other@example.com", "broken@"} {
		in := info()
		in.Email = email
		if !errors.Is(in.validate(), ErrInvalidInfo) {
			t.Fatalf("checkout accepted malformed address %q", email)
		}
		if _, err := db.Exec(`UPDATE "order" SET email=? WHERE public_id='guest-order'`, email); err != nil {
			t.Fatal(err)
		}
		if err := s.RequestGuestAccess(context.Background(), "guest-order", email, guestBrowser(t)); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid legacy email queued a code")
	}
}
