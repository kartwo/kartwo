// 运单通知状态测试 / Shipment Notice State Tests
// 功能：验证发货原子性、版本冲突、历史保护、未知结果核对与幂等重试并发
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:42:37
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"
)

func shipmentFixture(t *testing.T) (*sql.DB, *Service, string) {
	t.Helper()
	db, s, crt, cat := setup(t)
	ctx := context.Background()
	v := seedVariant(t, cat, "shipment-test", 2500, 20)
	cartID, _, err := crt.GetOrCreate(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = crt.AddItem(ctx, cartID, v, 1); err != nil {
		t.Fatal(err)
	}
	id, err := s.Checkout(ctx, cartID, info())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE "order" SET status='paid' WHERE public_id=?`, id); err != nil {
		t.Fatal(err)
	}
	return db, s, id
}

func assertShipmentCount(t *testing.T, db *sql.DB, shipments, revisions, notices, outbox int) {
	t.Helper()
	for table, want := range map[string]int{"shipment": shipments, "shipment_revision": revisions, "shipment_notice": notices, "email_outbox": outbox} {
		// table is a fixed test-only identifier, never external input.
		var got int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil || got != want {
			t.Fatalf("%s count=%d want=%d err=%v", table, got, want, err)
		}
	}
}
func mustShipmentHistory(t *testing.T, s *Service, id string) ShipmentHistory {
	t.Helper()
	h, err := s.ShipmentHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func shipFixture(t *testing.T, db *sql.DB, s *Service, id string) ShipmentHistory {
	t.Helper()
	if err := s.Fulfill(context.Background(), id, "Carrier", "TRACK-1", "https://carrier.example/track/1"); err != nil {
		t.Fatal(err)
	}
	assertShipmentCount(t, db, 1, 1, 1, 1)
	return mustShipmentHistory(t, s, id)
}
func noticeState(t *testing.T, db *sql.DB, id, status string, old bool) {
	t.Helper()
	if _, err := db.Exec(`UPDATE email_outbox SET status=?,attempts=5,last_error='private SMTP echo: never return this' WHERE id=(SELECT outbox_id FROM shipment_notice WHERE public_id=?)`, status, id); err != nil {
		t.Fatal(err)
	}
	if old {
		if _, err := db.Exec(`UPDATE shipment_notice SET created_at='2000-01-01T00:00:00.000Z' WHERE public_id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestShipmentAtomicEnqueueAndReplay(t *testing.T) {
	db, s, id := shipmentFixture(t)
	ctx := context.Background()
	if err := s.Fulfill(ctx, id, " Carrier ", " TRACK-1 ", "https://carrier.example/track/1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Fulfill(ctx, id, "Carrier", "TRACK-1", "https://carrier.example/track/1"); err != nil {
		t.Fatal("identical lost-response replay", err)
	}
	if err := s.Fulfill(ctx, id, "Carrier", "OTHER", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("second shipment must be explicit edit", err)
	}
	assertShipmentCount(t, db, 1, 1, 1, 1)
	h := mustShipmentHistory(t, s, id)
	if !h.HasShipment || h.CurrentVersion != 1 || len(h.Notices) != 1 || h.Notices[0].Status != "pending" || h.Notices[0].Origin != "shipped" {
		t.Fatalf("history %+v", h)
	}
	for _, publicID := range []string{h.CurrentRevision, h.Notices[0].PublicID} {
		parsed, err := uuid.Parse(publicID)
		if err != nil || parsed.Version() != 7 {
			t.Fatalf("ID is not UUIDv7: %s", publicID)
		}
	}
	var subject, body string
	if err := db.QueryRow(`SELECT subject,body FROM email_outbox`).Scan(&subject, &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(subject, "has shipped") || !strings.Contains(body, id) || !strings.Contains(body, "TRACK-1") || !strings.Contains(body, "Tracking version: 1") || strings.Contains(body, info().Address) {
		t.Fatal("notice must contain tracking and no shipping address")
	}
}

func TestShipmentRollbackIfNoticeCannotBeSaved(t *testing.T) {
	db, s, id := shipmentFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_notice BEFORE INSERT ON shipment_notice BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Fulfill(context.Background(), id, "Carrier", "TRACK-1", ""); err == nil {
		t.Fatal("expected atomic failure")
	}
	assertShipmentCount(t, db, 0, 0, 0, 0)
	o, err := s.Get(context.Background(), id)
	if err != nil || o.Status != "paid" {
		t.Fatal("partial fulfillment committed", err)
	}
}

func TestShipmentInputAndPaymentBarriers(t *testing.T) {
	db, s, id := shipmentFixture(t)
	ctx := context.Background()
	for _, fields := range []ShipmentFields{
		{"", "TRACK", ""}, {strings.Repeat("x", 129), "TRACK", ""}, {"Carrier", strings.Repeat("x", 129), ""}, {"Carrier\nBcc: x", "TRACK", ""}, {"Carrier", "TRACK\u2028more", ""},
		{"Carrier", "TRACK", "http://carrier.example"}, {"Carrier", "TRACK", "https://user:pass@carrier.example"}, {"Carrier", "TRACK", "javascript:alert(1)"}, {"Carrier", "TRACK", "https://carrier.example/a\\b"}, {"Carrier", "TRACK", "https://carrier.example/\n"},
	} {
		if err := s.Fulfill(ctx, id, fields.Carrier, fields.Number, fields.URL); !errors.Is(err, ErrInvalidInfo) {
			t.Fatalf("invalid fields accepted: %+v %v", fields, err)
		}
	}
	assertShipmentCount(t, db, 0, 0, 0, 0)
	if _, err := db.Exec(`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency,review_reason) SELECT id,'stripe','receipt-1',total_cents,currency,'duplicate_payment' FROM "order" WHERE public_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.Fulfill(ctx, id, "Carrier", "TRACK", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("unresolved money review allowed shipment", err)
	}
	if _, err := db.Exec(`UPDATE payment_receipt SET review_reason='',refunded_cents=amount_cents`); err != nil {
		t.Fatal(err)
	}
	if err := s.Fulfill(ctx, id, "Carrier", "TRACK", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("fully refunded receipt allowed shipment", err)
	}
	assertShipmentCount(t, db, 0, 0, 0, 0)
}

func TestShipmentVersionUpdateSupersedesAndRejectsStaleWrite(t *testing.T) {
	db, s, id := shipmentFixture(t)
	old := shipFixture(t, db, s, id)
	ctx := context.Background()
	if err := s.UpdateShipment(ctx, id, old.CurrentRevision, "New carrier", "TRACK-2", "https://carrier.example/2"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateShipment(ctx, id, old.CurrentRevision, "New carrier", "TRACK-2", "https://carrier.example/2"); err != nil {
		t.Fatal("lost update response replay", err)
	}
	if err := s.UpdateShipment(ctx, id, old.CurrentRevision, "Old carrier", "TRACK-3", ""); !errors.Is(err, ErrShipmentChanged) {
		t.Fatal("stale overwrite accepted", err)
	}
	assertShipmentCount(t, db, 1, 2, 2, 2)
	h := mustShipmentHistory(t, s, id)
	if h.CurrentVersion != 2 || h.Revisions[1].Number != "TRACK-1" || h.Revisions[0].Number != "TRACK-2" || h.Notices[0].Origin != "tracking_updated" || h.Notices[1].Status != "superseded" || h.Notices[1].Resolution != "superseded" {
		t.Fatalf("lost history %+v", h)
	}
	o, err := s.Get(ctx, id)
	if err != nil || o.TrackingNumber != "TRACK-2" || o.Status != "fulfilled" {
		t.Fatal("latest projection incorrect", err)
	}
}

func TestShipmentSendingBlocksUpdateAndRefundedShipmentSurvives(t *testing.T) {
	db, s, id := shipmentFixture(t)
	h := shipFixture(t, db, s, id)
	ctx := context.Background()
	noticeState(t, db, h.Notices[0].PublicID, "sending", true)
	if err := s.UpdateShipment(ctx, id, h.CurrentRevision, "Carrier", "TRACK-2", ""); !errors.Is(err, ErrShipmentSending) {
		t.Fatal("changed a shipment during SMTP", err)
	}
	assertShipmentCount(t, db, 1, 1, 1, 1)
	noticeState(t, db, h.Notices[0].PublicID, "delivery_unknown", true)
	if _, err := db.Exec(`UPDATE "order" SET status='refunded' WHERE public_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateShipment(ctx, id, h.CurrentRevision, "Carrier", "TRACK-2", ""); err != nil {
		t.Fatal(err)
	}
	next := mustShipmentHistory(t, s, id)
	if next.Notices[1].Status != "delivery_unknown" || next.Notices[1].Resolution != "superseded" {
		t.Fatal("unknown SMTP evidence overwritten")
	}
	o, err := s.AdminGet(ctx, id)
	if err != nil || o.Status != "refunded" || o.FulfillmentStatus != "fulfilled" || o.TrackingNumber != "TRACK-2" {
		t.Fatal("tracking edit changed funds/physical state", err)
	}
}

func TestShipmentConcurrentUpdateAndFulfill(t *testing.T) {
	db, s, id := shipmentFixture(t)
	db.SetMaxOpenConns(4)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Fulfill(ctx, id, "Carrier", "TRACK-1", "") }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertShipmentCount(t, db, 1, 1, 1, 1)
	h := mustShipmentHistory(t, s, id)
	errs = make(chan error, 2)
	for _, number := range []string{"A", "B"} {
		wg.Add(1)
		go func(number string) {
			defer wg.Done()
			errs <- s.UpdateShipment(ctx, id, h.CurrentRevision, "Carrier", number, "")
		}(number)
	}
	wg.Wait()
	close(errs)
	winners, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			winners++
		} else if errors.Is(err, ErrShipmentChanged) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winners, conflicts)
	}
	assertShipmentCount(t, db, 1, 2, 2, 2)
}

func TestShipmentNoticeSafeRetryAndDurableReplay(t *testing.T) {
	db, s, id := shipmentFixture(t)
	h := shipFixture(t, db, s, id)
	ctx := context.Background()
	old := h.Notices[0].PublicID
	noticeState(t, db, old, "failed", true)
	if _, err := db.Exec(`UPDATE "order" SET email='changed-after-notice@example.com' WHERE public_id=?`, id); err != nil {
		t.Fatal(err)
	}
	job, err := s.ManageShipmentNotice(ctx, id, old, "retry-request-key-0001", "retry")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.ManageShipmentNotice(ctx, id, old, "retry-request-key-0001", "retry")
	if err != nil || again != job {
		t.Fatal("lost response replay duplicated job", err)
	}
	if _, err = s.ManageShipmentNotice(ctx, id, old, "retry-request-key-0001", "resend_after_check"); !errors.Is(err, ErrShipmentNoticeChanged) {
		t.Fatal("same key accepted different action", err)
	}
	if _, err = s.ManageShipmentNotice(ctx, id, old, "retry-request-key-0002", "retry"); !errors.Is(err, ErrShipmentNoticeChanged) {
		t.Fatal("old notice restarted again", err)
	}
	h = mustShipmentHistory(t, s, id)
	if h.Notices[0].PublicID != job || h.Notices[0].Attempts != 0 || h.Notices[0].Status != "pending" || h.Notices[1].Attempts != 5 || h.Notices[1].Status != "failed" || h.Notices[1].Resolution != "retried" || h.Notices[1].Reason != "delivery_failed" {
		t.Fatalf("retry evidence %+v", h.Notices)
	}
	assertShipmentCount(t, db, 1, 1, 2, 2)
	var recipients int
	if err := db.QueryRow(`SELECT COUNT(DISTINCT to_addr) FROM email_outbox`).Scan(&recipients); err != nil || recipients != 1 {
		t.Fatal("retry silently retargeted recipient", recipients, err)
	}
	if _, err = s.ManageShipmentNotice(ctx, "missing-order", old, "retry-request-key-0003", "retry"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cross-order notice", err)
	}
}

func TestShipmentNoticeUnknownRequiresExplicitCheck(t *testing.T) {
	for _, action := range []string{"received", "resend_after_check"} {
		t.Run(action, func(t *testing.T) {
			db, s, id := shipmentFixture(t)
			h := shipFixture(t, db, s, id)
			ctx := context.Background()
			old := h.Notices[0].PublicID
			noticeState(t, db, old, "delivery_unknown", true)
			if _, err := s.ManageShipmentNotice(ctx, id, old, "unknown-request-key-01", "retry"); !errors.Is(err, ErrShipmentNoticeChanged) {
				t.Fatal("unknown result blindly retried", err)
			}
			job, err := s.ManageShipmentNotice(ctx, id, old, "unknown-request-key-02", action)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := s.ManageShipmentNotice(ctx, id, old, "unknown-request-key-02", action)
			if err != nil || replay != job {
				t.Fatal("unknown recovery replay", err)
			}
			h = mustShipmentHistory(t, s, id)
			original := h.Notices[len(h.Notices)-1]
			if original.Status != "delivery_unknown" || original.SentAt != "" || original.Attempts != 5 {
				t.Fatal("invented SMTP success", original)
			}
			if action == "received" {
				if len(h.Notices) != 1 || original.Resolution != "received" {
					t.Fatal("confirmation sent another mail")
				}
			} else if len(h.Notices) != 2 || original.Resolution != "not_received" || h.Notices[0].Origin != "resend_after_check" {
				t.Fatal("explicit resend not recorded")
			}
		})
	}
}

func TestShipmentNoticeStatusCooldownAndConcurrentRetry(t *testing.T) {
	db, s, id := shipmentFixture(t)
	h := shipFixture(t, db, s, id)
	ctx := context.Background()
	notice := h.Notices[0].PublicID
	for _, status := range []string{"pending", "sent", "superseded"} {
		noticeState(t, db, notice, status, false)
		if _, err := s.ManageShipmentNotice(ctx, id, notice, "status-request-key-01", "retry"); !errors.Is(err, ErrShipmentNoticeChanged) {
			t.Fatalf("status %s retried: %v", status, err)
		}
	}
	noticeState(t, db, notice, "sending", false)
	if _, err := s.ManageShipmentNotice(ctx, id, notice, "status-request-key-01", "retry"); !errors.Is(err, ErrShipmentSending) {
		t.Fatal(err)
	}
	noticeState(t, db, notice, "failed", false)
	if _, err := s.ManageShipmentNotice(ctx, id, notice, "status-request-key-01", "retry"); !errors.Is(err, ErrShipmentNoticeCooldown) {
		t.Fatal("cooldown missing", err)
	}
	noticeState(t, db, notice, "failed", true)
	db.SetMaxOpenConns(4)
	var wg sync.WaitGroup
	type result struct {
		id  string
		err error
	}
	results := make(chan result, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := s.ManageShipmentNotice(ctx, id, notice, "status-request-key-02", "retry")
			results <- result{job, err}
		}()
	}
	wg.Wait()
	close(results)
	first := ""
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if first == "" {
			first = result.id
		}
		if first != result.id {
			t.Fatal("concurrent retry created multiple notices")
		}
	}
	assertShipmentCount(t, db, 1, 1, 2, 2)
}

func TestShipmentMigrationPreservesLegacyWithoutSending(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/legacy.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	old := fstest.MapFS{}
	files, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	pending := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if file.Name() >= "0036" {
			pending++
			continue
		}
		body, err := fs.ReadFile(migrations.FS, file.Name())
		if err != nil {
			t.Fatal(err)
		}
		old[file.Name()] = &fstest.MapFile{Data: body}
	}
	if _, err = migrate.Run(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO customer(id,public_id,email,name) VALUES(1,'legacy-customer','legacy@example.com','Legacy')`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) VALUES(1,'legacy-order',1,'fulfilled','legacy@example.com','Legacy customer','Legacy address','USD',2500,2500)`,
		`INSERT INTO shipment(id,order_id,carrier,tracking_number,tracking_url,shipped_at) VALUES(1,1,'Legacy carrier','LEGACY','https://carrier.example/old','2026-01-01T00:00:00.000Z')`,
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := migrate.Run(ctx, db, migrations.FS); err != nil || n != pending {
		t.Fatalf("upgrade=%d err=%v", n, err)
	}
	s := New(db, nil)
	h := mustShipmentHistory(t, s, "legacy-order")
	if !h.HasShipment || h.CurrentVersion != 0 || h.CurrentRevision != "" {
		t.Fatal("fabricated legacy version", h)
	}
	assertShipmentCount(t, db, 1, 0, 0, 0)
	if err = s.UpdateShipment(ctx, "legacy-order", "", "New carrier", "NEW", ""); err != nil {
		t.Fatal(err)
	}
	h = mustShipmentHistory(t, s, "legacy-order")
	if h.CurrentVersion != 2 || !h.Revisions[1].Legacy || h.Revisions[1].CreatedAt != "2026-01-01T00:00:00.000Z" || len(h.Notices) != 1 || h.Notices[0].Origin != "tracking_updated" {
		t.Fatal("legacy snapshot lost", h)
	}
	assertShipmentCount(t, db, 1, 2, 1, 1)
	if n, err := migrate.Run(ctx, db, migrations.FS); err != nil || n != 0 {
		t.Fatal("non-reentrant migration", n, err)
	}
}

func TestShipmentUpdateAndRetryRollbackPreserveEvidence(t *testing.T) {
	for _, operation := range []string{"update", "retry"} {
		t.Run(operation, func(t *testing.T) {
			db, s, id := shipmentFixture(t)
			h := shipFixture(t, db, s, id)
			ctx := context.Background()
			if operation == "retry" {
				noticeState(t, db, h.Notices[0].PublicID, "failed", true)
			}
			if _, err := db.Exec(`CREATE TRIGGER reject_followup BEFORE INSERT ON shipment_notice BEGIN SELECT RAISE(ABORT,'followup failure'); END`); err != nil {
				t.Fatal(err)
			}
			var err error
			if operation == "update" {
				err = s.UpdateShipment(ctx, id, h.CurrentRevision, "Changed", "NEW", "")
			} else {
				_, err = s.ManageShipmentNotice(ctx, id, h.Notices[0].PublicID, "rollback-request-key-01", "retry")
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			assertShipmentCount(t, db, 1, 1, 1, 1)
			after := mustShipmentHistory(t, s, id)
			if after.CurrentRevision != h.CurrentRevision || after.Revisions[0].Number != "TRACK-1" || after.Notices[0].Resolution != "" {
				t.Fatal("partial followup committed", after)
			}
			if operation == "update" && after.Notices[0].Status != "pending" {
				t.Fatal("old queue incorrectly superseded")
			}
			if operation == "retry" && (after.Notices[0].Status != "failed" || after.Notices[0].Attempts != 5) {
				t.Fatal("old failure evidence lost")
			}
		})
	}
}

func TestShipmentHistoryBoundedWithoutDroppingCounts(t *testing.T) {
	db, s, id := shipmentFixture(t)
	h := shipFixture(t, db, s, id)
	ctx := context.Background()
	for i := 0; i < 24; i++ {
		if err := s.UpdateShipment(ctx, id, h.CurrentRevision, "Carrier", fmt.Sprintf("TRACK-%d", i+2), ""); err != nil {
			t.Fatal(err)
		}
		h = mustShipmentHistory(t, s, id)
	}
	for i := 0; i < 30; i++ {
		current := h.Notices[0].PublicID
		noticeState(t, db, current, "failed", true)
		if _, err := s.ManageShipmentNotice(ctx, id, current, fmt.Sprintf("history-request-%016d", i), "retry"); err != nil {
			t.Fatal(err)
		}
		h = mustShipmentHistory(t, s, id)
	}
	if h.RevisionCount != 25 || len(h.Revisions) != 20 || h.NoticeCount != 55 || len(h.Notices) != 50 || h.CurrentVersion != 25 || h.Notices[0].Sequence != 31 {
		t.Fatal("history bounds/counts incorrect", h)
	}
}
