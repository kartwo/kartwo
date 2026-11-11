// 恢复访问凭证测试 / Restore Access Credential Tests
// 功能：恢复不复活临时授权，保留业务与加密配置，旧库兼容及原子失败隔离
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 23:48:55
package backup

import (
	"context"
	"database/sql"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"
	"os"
	"path/filepath"
	"testing"
)

func restoreAccessSource(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db")+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err = migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO admin_user(id,public_id,username,password_hash) VALUES(1,'restore-owner','restore-owner','original-password-hash')`,
		`INSERT INTO session(token,admin_id,csrf_token,expires_at) VALUES('restore-old-session',1,'restore-old-csrf','2099-01-01T00:00:00.000Z')`,
		`INSERT INTO admin_invite(token_hash,admin_id,expires_at) VALUES('restore-old-invite-digest',1,'2099-01-01T00:00:00.000Z')`,
		`INSERT INTO customer(id,public_id,email) VALUES(1,'restore-customer','restore@example.invalid')`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) VALUES(1,'restore-order',1,'paid','restore@example.invalid','Virtual return buyer','Virtual original address','USD',1200,1200)`,
		`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency) VALUES(1,'stripe','restore-payment',1200,'USD')`,
		`INSERT INTO after_sale(id,public_id,order_id,client_key,kind,state,version,amount_cents,currency,reason,policy_configured) VALUES(1,'restore-sale',1,'restore-operation','refund','requested',1,123,'USD','Preserve original request',0)`,
		`INSERT INTO after_sale_event(after_sale_id,version,client_key,request_hash,actor,action,state,message) VALUES(1,1,'restore-operation','restore-body-digest','customer','request','requested','Preserve original request')`,
		`INSERT INTO setting(key,value,encrypted) VALUES('restore.encrypted','original-ciphertext-fixture',1)`,
		`INSERT INTO order_guest_access(token_hash,order_id,expires_at) VALUES('restore-old-guest-digest',1,4102444800)`,
		`INSERT INTO email_outbox(id,order_id,kind,to_addr,subject,body,status) VALUES(1,1,'guest_access:restore-pending','restore@example.invalid','Code','pending-private-code','pending'),(2,1,'guest_access:restore-sending','restore@example.invalid','Code','sending-private-code','sending'),(3,1,'guest_access:restore-sent','restore@example.invalid','Code','sent-private-code','sent'),(4,1,'guest_access:restore-unknown','restore@example.invalid','Code','unknown-private-code','delivery_unknown'),(5,1,'order_confirmation','restore@example.invalid','Order confirmed','Preserve original business mail','pending')`,
		`INSERT INTO order_guest_challenge(public_id,order_id,browser_hash,code_hash,outbox_id,expires_at,requested_at) VALUES('restore-pending',1,'restore-browser','restore-code-digest',1,4102444800,12345)`,
	}
	for _, q := range statements {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	archive, _, err := New(db, dir, "restore-access-fixture").Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return db, archive, dir
}
func restoreAccessCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil || n != want {
		t.Fatalf("restore count=%d want=%d err=%v", n, want, err)
	}
}
func TestRestoreInvalidatesTemporaryAccessWithoutChangingBusiness(t *testing.T) {
	source, archive, _ := restoreAccessSource(t)
	target := filepath.Join(t.TempDir(), "restored")
	if _, err := Restore(archive, target); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(target, "shop.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	restoreAccessCount(t, db, `SELECT count(*) FROM session`, 0)
	restoreAccessCount(t, db, `SELECT count(*) FROM admin_invite`, 0)
	restoreAccessCount(t, db, `SELECT count(*) FROM order_guest_access`, 0)
	restoreAccessCount(t, db, `SELECT count(*) FROM order_guest_challenge WHERE consumed_at=0`, 0)
	restoreAccessCount(t, db, `SELECT count(*) FROM order_guest_challenge WHERE requested_at=12345 AND code_hash='restore-code-digest'`, 1)
	restoreAccessCount(t, db, `SELECT count(*) FROM email_outbox WHERE kind GLOB 'guest_access:*' AND body<>''`, 0)
	restoreAccessCount(t, db, `SELECT count(*) FROM email_outbox WHERE id IN (1,2) AND status='skipped'`, 2)
	restoreAccessCount(t, db, `SELECT count(*) FROM email_outbox WHERE id=3 AND status='sent'`, 1)
	restoreAccessCount(t, db, `SELECT count(*) FROM email_outbox WHERE id=4 AND status='delivery_unknown'`, 1)
	for query, want := range map[string]int{
		`SELECT count(*) FROM admin_user WHERE password_hash='original-password-hash'`:                                 1,
		`SELECT count(*) FROM setting WHERE encrypted=1 AND value='original-ciphertext-fixture'`:                       1,
		`SELECT count(*) FROM "order" WHERE total_cents=1200 AND ship_address='Virtual original address'`:              1,
		`SELECT count(*) FROM payment_receipt WHERE amount_cents=1200 AND refunded_cents=0`:                            1,
		`SELECT count(*) FROM after_sale WHERE client_key='restore-operation' AND state='requested'`:                   1,
		`SELECT count(*) FROM after_sale_event WHERE request_hash='restore-body-digest'`:                               1,
		`SELECT count(*) FROM email_outbox WHERE id=5 AND status='pending' AND body='Preserve original business mail'`: 1,
		`SELECT count(*) FROM schema_migrations`:                                                                       38,
		`SELECT count(*) FROM admin_role`:                                                                              4,
	} {
		restoreAccessCount(t, db, query, want)
	}
	// Source DB and its active authorization are untouched, including the plaintext
	// code that remains eligible there. Restoration only modifies its private stage.
	restoreAccessCount(t, source, `SELECT count(*) FROM session`, 1)
	restoreAccessCount(t, source, `SELECT count(*) FROM admin_invite`, 1)
	restoreAccessCount(t, source, `SELECT count(*) FROM order_guest_access`, 1)
	restoreAccessCount(t, source, `SELECT count(*) FROM order_guest_challenge WHERE consumed_at=0`, 1)
	restoreAccessCount(t, source, `SELECT count(*) FROM email_outbox WHERE id=1 AND body='pending-private-code'`, 1)
}
func TestRestoreAccessCleanupFailureNeverPublishesTarget(t *testing.T) {
	for _, failure := range []string{"ABORT", "IGNORE"} {
		t.Run(failure, func(t *testing.T) {
			source, _, dir := restoreAccessSource(t)
			trigger := `CREATE TRIGGER fail_restore_cleanup BEFORE DELETE ON session BEGIN SELECT RAISE(IGNORE); END`
			if failure == "ABORT" {
				trigger = `CREATE TRIGGER fail_restore_cleanup BEFORE DELETE ON session BEGIN SELECT RAISE(ABORT,'fixture cleanup failure'); END`
			}
			if _, err := source.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			archive, _, err := New(source, dir, "restore-access-failure").Create(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			target := filepath.Join(parent, "restored")
			if _, err = Restore(archive, target); err == nil {
				t.Fatal("unsafe restored target published after credential cleanup failed")
			}
			if _, err = os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("failed target exists")
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("private restore stage left behind", err)
			}
			restoreAccessCount(t, source, `SELECT count(*) FROM session`, 1)
		})
	}
}
func TestRestoreOldSchemaKeepsSchemaAndHasBoundedCredentialCleanup(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, q := range []string{`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT)`, `INSERT INTO meta VALUES('original','old-schema')`, `CREATE TABLE SESSION(token TEXT)`, `INSERT INTO session VALUES('old-legacy-session')`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	archive, _, err := New(db, dir, "legacy").Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err = Restore(archive, target); err != nil {
		t.Fatal(err)
	}
	copy, err := sql.Open("sqlite", "file:"+filepath.Join(target, "shop.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copy.Close() }()
	restoreAccessCount(t, copy, `SELECT count(*) FROM session`, 0)
	restoreAccessCount(t, copy, `SELECT count(*) FROM meta WHERE value='old-schema'`, 1)
	restoreAccessCount(t, copy, `SELECT count(*) FROM sqlite_master WHERE type='table'`, 2)
	for _, name := range []string{"shop.db-wal", "shop.db-journal"} {
		if _, err = os.Stat(filepath.Join(target, name)); !os.IsNotExist(err) {
			t.Fatalf("restore left a required transient sidecar: %s", name)
		}
	}
}

func TestRestoreRejectsCredentialViewInsteadOfIgnoringAuthorization(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE VIEW session AS SELECT 'historical-session' AS token`); err != nil {
		t.Fatal(err)
	}
	archive, _, err := New(db, dir, "invalid-auth-schema").Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err = Restore(archive, target); err == nil {
		t.Fatal("restored credential view bypassed invalidation")
	}
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("unsafe restore target published")
	}
}
