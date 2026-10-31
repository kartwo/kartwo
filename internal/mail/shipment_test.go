// 发货邮件版本测试 / Shipment Mail Revision Tests
// 功能：验证不可变通知正文、旧版本与旧重试作废、队列校验失败关闭及未配置跳过
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:44:07
package mail

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

func TestShipmentCompositionAndTransactionalEnqueue(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	info := ShipmentInfo{OrderID: 7, OrderPublicID: "order-public-id", NoticeID: uuid.Must(uuid.NewV7()).String(), Email: "buyer@example.com", Version: 2, Carrier: "Carrier", Number: "TRACK-2", URL: "https://carrier.example/2", ChangedAt: "2026-10-25T00:00:00.000Z"}
	if _, err = EnqueueShipment(ctx, sqlcgen.New(tx), info); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatal("enqueue escaped transaction", count, err)
	}
	id, err := EnqueueShipment(ctx, sqlcgen.New(db), info)
	if err != nil {
		t.Fatal(err)
	}
	var kind, subject, body string
	if err = db.QueryRow(`SELECT kind,subject,body FROM email_outbox WHERE id=?`, id).Scan(&kind, &subject, &body); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Tracking version: 2", "TRACK-2", info.URL, info.ChangedAt, "replaces any earlier tracking details"} {
		if !strings.Contains(body, text) {
			t.Fatalf("notice missing %q", text)
		}
	}
	if kind != ShipmentKindPrefix+info.NoticeID || !strings.Contains(subject, "Tracking update") || strings.Contains(body, info.Email) {
		t.Fatal("incorrect notice identity/privacy")
	}
	if _, err = EnqueueShipment(ctx, sqlcgen.New(db), info); err == nil {
		t.Fatal("duplicate immutable notice silently accepted")
	}
	info.NoticeID = uuid.Must(uuid.NewV7()).String()
	info.OrderPublicID = "header\r\nBcc: injected"
	if _, err = EnqueueShipment(ctx, sqlcgen.New(db), info); err == nil {
		t.Fatal("header injection accepted")
	}
}

// Build queued snapshots directly: worker tests do not depend on order service
// internals, and fake SMTP never contacts a network endpoint.
func shipmentQueue(t *testing.T, db *sql.DB, sameRevision bool) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO shipment(id,order_id,carrier,tracking_number,tracking_url) VALUES(1,7,'Carrier','CURRENT','')`); err != nil {
		t.Fatal(err)
	}
	q := sqlcgen.New(db)
	ids := []int64{}
	for i := 1; i <= 2; i++ {
		revisionID := int64(i)
		version := int64(i)
		number := "OLD"
		if i == 2 {
			number = "CURRENT"
		}
		if sameRevision {
			revisionID = 1
			version = 1
			number = "CURRENT"
		}
		if i == 1 || !sameRevision {
			if _, err := db.Exec(`INSERT INTO shipment_revision(id,public_id,shipment_id,version,carrier,tracking_number) VALUES(?,?,1,?,'Carrier',?)`, revisionID, uuid.Must(uuid.NewV7()).String(), version, number); err != nil {
				t.Fatal(err)
			}
		}
		noticeID := uuid.Must(uuid.NewV7()).String()
		outboxID, err := EnqueueShipment(ctx, q, ShipmentInfo{OrderID: 7, OrderPublicID: "order-fixture", NoticeID: noticeID, Email: "buyer@example.com", Version: version, Carrier: "Carrier", Number: number, ChangedAt: "2026-10-25T00:00:00.000Z"})
		if err != nil {
			t.Fatal(err)
		}
		sequence := 1
		if sameRevision {
			sequence = i
		}
		if _, err = db.Exec(`INSERT INTO shipment_notice(public_id,revision_id,sequence,request_key,origin,outbox_id) VALUES(?,?,?,?,'shipped',?)`, noticeID, revisionID, sequence, noticeID, outboxID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, outboxID)
	}
	return ids[0], ids[1]
}

func TestShipmentWorkerRejectsOldRevisionAndRetryCopy(t *testing.T) {
	for _, sameRevision := range []bool{false, true} {
		t.Run(map[bool]string{false: "old revision", true: "old retry copy"}[sameRevision], func(t *testing.T) {
			db := newDB(t)
			old, current := shipmentQueue(t, db, sameRevision)
			w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
			sends := 0
			w.send = func(_ Config, _ string, _ string, body string) error {
				sends++
				if strings.Contains(body, "Tracking number: OLD") {
					t.Fatal("sent obsolete tracking")
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var status string
				if err := db.QueryRowContext(ctx, `SELECT status FROM email_outbox WHERE id=?`, current).Scan(&status); err != nil || status != "sending" {
					t.Fatal("SMTP held DB connection or was not claimed", err)
				}
				return nil
			}
			w.tick(context.Background())
			w.tick(context.Background())
			status, _ := rowState(t, db, old)
			currentStatus, _ := rowState(t, db, current)
			if status != "superseded" || currentStatus != "sent" || sends != 1 {
				t.Fatalf("old=%s current=%s sends=%d", status, currentStatus, sends)
			}
		})
	}
}

func TestShipmentWorkerBindingAndMissingVersionFailClosed(t *testing.T) {
	for _, broken := range []string{"order binding", "kind binding", "missing table"} {
		t.Run(broken, func(t *testing.T) {
			db := newDB(t)
			_, current := shipmentQueue(t, db, false)
			switch broken {
			case "order binding":
				if _, err := db.Exec(`UPDATE email_outbox SET order_id=99 WHERE id=?`, current); err != nil {
					t.Fatal(err)
				}
			case "kind binding":
				if _, err := db.Exec(`UPDATE email_outbox SET kind='shipment:wrong-identity' WHERE id=?`, current); err != nil {
					t.Fatal(err)
				}
			case "missing table":
				if _, err := db.Exec(`DROP TABLE shipment_revision`); err != nil {
					t.Fatal(err)
				}
			}
			w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
			w.send = func(Config, string, string, string) error { t.Fatal("invalid binding reached SMTP"); return nil }
			w.tick(context.Background())
			status, attempts := rowState(t, db, current)
			if broken == "missing table" {
				if status != "pending" || attempts != 0 {
					t.Fatal("read failure treated as send attempt", status, attempts)
				}
			} else if status != "superseded" {
				t.Fatal("invalid binding retained", status)
			}
		})
	}
}

func TestShipmentWorkerUnconfiguredSkipsOnlyCurrentNotice(t *testing.T) {
	db := newDB(t)
	old, current := shipmentQueue(t, db, false)
	w := NewWorker(db, NewCache(settings.New(db)), quietLogger(), time.Second)
	w.send = func(Config, string, string, string) error { t.Fatal("unconfigured SMTP attempted"); return nil }
	w.tick(context.Background())
	status, _ := rowState(t, db, old)
	currentStatus, attempts := rowState(t, db, current)
	if status != "superseded" || currentStatus != "skipped" || attempts != 0 {
		t.Fatal(status, currentStatus, attempts)
	}
}

func TestShipmentWorkerLostResultWriteBecomesUnknownWithoutResend(t *testing.T) {
	db := newDB(t)
	_, current := shipmentQueue(t, db, false)
	if _, err := db.Exec(`CREATE TRIGGER reject_sent BEFORE UPDATE OF status ON email_outbox WHEN NEW.status='sent' BEGIN SELECT RAISE(ABORT,'fixture result write lost'); END`); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
	sends := 0
	w.send = func(Config, string, string, string) error {
		sends++
		var claimed int64
		if err := db.QueryRow(`SELECT sending_at FROM shipment_notice WHERE outbox_id=?`, current).Scan(&claimed); err != nil || claimed <= 0 {
			t.Fatal("claim clock not committed", claimed, err)
		}
		// Simulate elapsed time without delaying the test or changing wall clock.
		if _, err := db.Exec(`UPDATE shipment_notice SET sending_at=? WHERE outbox_id=?`, time.Now().Add(-3*time.Minute).Unix(), current); err != nil {
			t.Fatal(err)
		}
		return nil
	}
	w.tick(context.Background())
	status, _ := rowState(t, db, current)
	if status != "sending" || sends != 1 {
		t.Fatal("lost result not retained", status, sends)
	}
	w.tick(context.Background())
	w.tick(context.Background())
	status, attempts := rowState(t, db, current)
	if status != "delivery_unknown" || attempts != 1 || sends != 1 {
		t.Fatal("orphan result blindly resent", status, attempts, sends)
	}
}

func TestShipmentWorkerDoesNotExpireFreshSMTPClaim(t *testing.T) {
	db := newDB(t)
	_, current := shipmentQueue(t, db, false)
	if _, err := db.Exec(`UPDATE email_outbox SET status='sending' WHERE id=?`, current); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE shipment_notice SET sending_at=? WHERE outbox_id=?`, time.Now().Unix(), current); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
	w.send = func(Config, string, string, string) error { t.Fatal("fresh claim duplicated"); return nil }
	w.tick(context.Background())
	status, attempts := rowState(t, db, current)
	if status != "sending" || attempts != 0 {
		t.Fatal("fresh claim prematurely expired", status, attempts)
	}
}
