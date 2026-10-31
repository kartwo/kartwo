// 验证码邮件生命周期测试 / Verification Mail Lifecycle Tests
// 功能：验证有效挑战才发送、锁定时仍清理过期码、发送后清除正文和错误脱敏
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package mail

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func enqueueGuestCode(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO email_outbox(order_id,kind,to_addr,subject,body) VALUES(1,'guest_access:test-challenge','private@example.com','Access code','transient-code-12345678')`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO order_guest_challenge(public_id,order_id,browser_hash,code_hash,outbox_id,expires_at,requested_at) VALUES('test-challenge',1,'browser-digest','code-digest',?,?,?)`, id, time.Now().Add(10*time.Minute).Unix(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	return id
}
func assertGuestMailErased(t *testing.T, db *sql.DB, id int64, state string) {
	t.Helper()
	var body, status string
	if err := db.QueryRow(`SELECT body,status FROM email_outbox WHERE id=?`, id).Scan(&body, &status); err != nil {
		t.Fatal(err)
	}
	if body != "" || status != state {
		t.Fatalf("code body empty=%v status=%s expected=%s", body == "", status, state)
	}
}
func TestGuestMailOnlyValidChallengesAreSent(t *testing.T) {
	for _, state := range []string{"valid", "expired", "consumed", "exhausted", "orphan"} {
		t.Run(state, func(t *testing.T) {
			db := newDB(t)
			id := enqueueGuestCode(t, db)
			cache := dbConfiguredCache(t, db)
			var query string
			switch state {
			case "expired":
				query = `UPDATE order_guest_challenge SET expires_at=1`
			case "consumed":
				query = `UPDATE order_guest_challenge SET consumed_at=1`
			case "exhausted":
				query = `UPDATE order_guest_challenge SET attempts=5`
			case "orphan":
				query = `DELETE FROM order_guest_challenge`
			}
			if query != "" {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			w := NewWorker(db, cache, quietLogger(), time.Second)
			calls := 0
			w.send = func(_ Config, to, _, body string) error {
				calls++
				if to != "private@example.com" || body != "transient-code-12345678" {
					t.Error("incorrect code delivery")
				}
				return nil
			}
			w.tick(context.Background())
			want := 0
			status := "skipped"
			if state == "valid" {
				want = 1
				status = "sent"
			}
			if calls != want {
				t.Fatalf("SMTP calls=%d want=%d", calls, want)
			}
			assertGuestMailErased(t, db, id, status)
		})
	}
}
func TestGuestMailLockedCleanupAndSafeErrors(t *testing.T) {
	t.Run("locked expiration", func(t *testing.T) {
		db := newDB(t)
		id := enqueueGuestCode(t, db)
		cache := dbConfiguredCache(t, db)
		cache.Lock()
		if _, err := db.Exec(`UPDATE order_guest_challenge SET expires_at=1`); err != nil {
			t.Fatal(err)
		}
		w := NewWorker(db, cache, quietLogger(), time.Second)
		w.send = func(Config, string, string, string) error { t.Fatal("locked SMTP sent code"); return nil }
		w.tick(context.Background())
		assertGuestMailErased(t, db, id, "skipped")
	})
	t.Run("error redaction and retry", func(t *testing.T) {
		db := newDB(t)
		id := enqueueGuestCode(t, db)
		var output bytes.Buffer
		w := NewWorker(db, dbConfiguredCache(t, db), slog.New(slog.NewTextHandler(&output, nil)), time.Second)
		w.send = func(Config, string, string, string) error {
			return errors.New("private@example.com transient-code-12345678 smtp-credential")
		}
		w.tick(context.Background())
		var savedError, body string
		if err := db.QueryRow(`SELECT last_error,body FROM email_outbox WHERE id=?`, id).Scan(&savedError, &body); err != nil {
			t.Fatal(err)
		}
		if savedError != "verification email delivery failed" || body == "" {
			t.Fatal("temporary delivery failure must safely retain a retryable code")
		}
		for _, secret := range []string{"private@example.com", "12345678", "smtp-credential"} {
			if strings.Contains(output.String(), secret) || strings.Contains(savedError, secret) {
				t.Fatal("SMTP error leaked private data")
			}
		}
		if _, err := db.Exec(`UPDATE email_outbox SET attempts=4,next_attempt_at='2000-01-01T00:00:00.000Z' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		w.tick(context.Background())
		assertGuestMailErased(t, db, id, "failed")
	})
}
