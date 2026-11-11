// 售后核心边界测试 / After Sale Domain Tests
// 功能：申请授权、政策快照、审核/退回状态机、并发幂等、退款证据、发货阻断及升级重入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 20:59:28
package order

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/policy"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"
)

type afterSaleFixture struct {
	db           *sql.DB
	s            *Service
	order, token string
}

func saleFixture(t *testing.T, shipped bool) afterSaleFixture {
	t.Helper()
	db, s, crt, cat := setup(t)
	ctx := context.Background()
	v := seedVariant(t, cat, "after-sale-product", 2500, 5)
	cartID, _, err := crt.GetOrCreate(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = crt.AddItem(ctx, cartID, v, 1); err != nil {
		t.Fatal(err)
	}
	id, token, err := s.CheckoutForGuest(ctx, cartID, info())
	if err != nil {
		t.Fatal(err)
	}
	// Domain fixture: ledger and stock finalization emulate an already verified
	// payment. Real signed HTTP payment integration is covered separately.
	for _, query := range []string{
		`UPDATE "order" SET status='paid' WHERE public_id=?`,
		`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency) SELECT id,'stripe','fixture-sale-payment',total_cents,currency FROM "order" WHERE public_id=?`,
	} {
		if _, err = db.Exec(query, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`UPDATE inventory SET quantity=quantity-reserved,reserved=0`); err != nil {
		t.Fatal(err)
	}
	if shipped {
		if err = s.Fulfill(ctx, id, "Fixture carrier", "OUTBOUND-1", ""); err != nil {
			t.Fatal(err)
		}
	}
	return afterSaleFixture{db, s, id, token}
}

func saleInput(kind string) AfterSaleInput {
	return AfterSaleInput{Key: uuid.Must(uuid.NewV7()).String(), Kind: kind, Reason: "The size does not fit.\nPlease review my request.", AmountCents: 1000}
}
func saleAction(version int64, action, message string) AfterSaleAction {
	return AfterSaleAction{Key: uuid.Must(uuid.NewV7()).String(), ExpectedVersion: version, Action: action, Message: message}
}
func saleCreate(t *testing.T, f afterSaleFixture, input AfterSaleInput) AfterSaleCase {
	t.Helper()
	c, err := f.s.CreateAfterSale(context.Background(), f.order, f.token, input)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func saleReview(t *testing.T, f afterSaleFixture, c AfterSaleCase, input AfterSaleAction) AfterSaleCase {
	t.Helper()
	c, err := f.s.ReviewAfterSale(context.Background(), f.order, c.PublicID, input)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func saleCount(t *testing.T, db *sql.DB, cases, events int) {
	t.Helper()
	var c, e int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM after_sale),(SELECT COUNT(*) FROM after_sale_event)`).Scan(&c, &e); err != nil || c != cases || e != events {
		t.Fatalf("cases=%d events=%d err=%v", c, e, err)
	}
}

func TestAfterSalePolicySnapshotAndUnconfiguredDefaults(t *testing.T) {
	f := saleFixture(t, true)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("return_refund"))
	if c.Terms.Configured || c.Terms.WindowDays != 0 || c.Terms.ShippingPayer != "" || c.Terms.SupportEmail != "" {
		t.Fatal("unconfigured demo defaults are not merchant policy")
	}
	c = saleReview(t, f, c, saleAction(c.Version, "reject", "Please contact support about this request."))
	p := policy.KartwoDemoProfile()
	p.SupportEmail = "returns@example.invalid"
	p.ReturnWindowDays = 21
	p.ReturnShippingPayer = "merchant"
	if err := policy.New(settings.New(f.db), nil).Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Shipping is not delivery. An old dispatch date cannot silently expire a
	// customer's return rights: this service has no confirmed receipt timestamp.
	if _, err := f.db.Exec(`UPDATE shipment SET shipped_at='2000-01-01T00:00:00.000Z'`); err != nil {
		t.Fatal(err)
	}
	c = saleCreate(t, f, saleInput("return_refund"))
	if !c.Terms.Configured || c.Terms.WindowDays != 21 || c.Terms.ShippingPayer != "merchant" || c.Terms.SupportEmail != p.SupportEmail {
		t.Fatal("policy was not captured")
	}
	p.ReturnWindowDays = 7
	p.ReturnShippingPayer = "buyer"
	if err := policy.New(settings.New(f.db), nil).Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	h, err := f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || len(h.Cases) != 2 || h.Cases[0].Terms.WindowDays != 21 || h.Cases[1].Terms.Configured || h.CanRequestRefund {
		t.Fatalf("immutable terms/active request: %+v %v", h, err)
	}
}

func TestAfterSaleAdmissionMoneyAndInputBounds(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	for _, mutate := range []func(*AfterSaleInput){
		func(i *AfterSaleInput) { i.Key = uuid.NewString() },
		func(i *AfterSaleInput) { i.Reason = " " },
		func(i *AfterSaleInput) { i.Reason = strings.Repeat("字", 668) },
		func(i *AfterSaleInput) { i.Reason = "unsafe\x00reason" },
		func(i *AfterSaleInput) { i.Reason = string([]byte{0xff}) },
		func(i *AfterSaleInput) { i.AmountCents = 0 },
		func(i *AfterSaleInput) { i.AmountCents = -1 },
		func(i *AfterSaleInput) { i.Kind = "exchange" },
	} {
		input := saleInput("refund")
		mutate(&input)
		if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); !errors.Is(err, ErrAfterSaleInvalid) {
			t.Fatalf("invalid accepted: %v", err)
		}
	}
	input := saleInput("return_refund")
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); !errors.Is(err, ErrAfterSaleIneligible) {
		t.Fatal("unshipped return accepted", err)
	}
	input = saleInput("refund")
	input.AmountCents = 3001
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); !errors.Is(err, ErrAfterSaleIneligible) {
		t.Fatal("excess amount", err)
	}
	if _, err := f.db.Exec(`UPDATE payment_receipt SET refunded_cents=2500`); err != nil {
		t.Fatal(err)
	}
	input.AmountCents = 501
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); !errors.Is(err, ErrAfterSaleIneligible) {
		t.Fatal("partial refund remaining limit", err)
	}
	input.AmountCents = 500
	c := saleCreate(t, f, input)
	if c.AmountCents != 500 || c.Currency != "USD" || c.State != "requested" {
		t.Fatalf("request %+v", c)
	}
	saleCount(t, f.db, 1, 1)
}

func TestAfterSaleDuplicateReceiptsCannotOverflowOrderLimit(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	if _, err := f.db.Exec(`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency,review_reason) SELECT id,'stripe','fixture-duplicate',9223372036854775807,currency,'duplicate_payment' FROM "order"`); err != nil {
		t.Fatal(err)
	}
	h, err := f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || h.MaxRequestCents != 3000 || !h.CanRequestRefund || h.CanRequestReturn {
		t.Fatalf("capped balance %+v %v", h, err)
	}
	if _, err = f.db.Exec(`UPDATE payment_receipt SET refunded_cents=amount_cents`); err != nil {
		t.Fatal(err)
	}
	h, err = f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || h.MaxRequestCents != 0 || h.CanRequestRefund {
		t.Fatal("fully refunded offered new request", err)
	}
	if _, err = f.db.Exec(`DELETE FROM payment_receipt`); err != nil {
		t.Fatal(err)
	}
	h, err = f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || h.MaxRequestCents != 3000 {
		t.Fatal("legacy paid review blocked", err)
	}
	if _, err = f.db.Exec(`UPDATE "order" SET status='pending'`); err != nil {
		t.Fatal(err)
	}
	h, err = f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || h.CanRequestRefund {
		t.Fatal("unpaid order offered request", err)
	}
}

func TestAfterSaleGuestScopeRevocationAndReplay(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	input := saleInput("refund")
	for _, token := range []string{"", guestBrowser(t)} {
		if _, err := f.s.CreateAfterSale(ctx, f.order, token, input); !errors.Is(err, ErrGuestVerification) {
			t.Fatal("unverified create", err)
		}
	}
	c := saleCreate(t, f, input)
	if _, err := f.db.Exec(`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) SELECT 'other-after-sale-order',customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents FROM "order" WHERE public_id=?`, f.order); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateAfterSale(ctx, "other-after-sale-order", f.token, saleInput("refund")); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("cross-order create", err)
	}
	if _, err := f.s.GuestAfterSales(ctx, "other-after-sale-order", f.token); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("cross-order read", err)
	}
	if _, err := f.db.Exec(`UPDATE order_guest_access SET expires_at=?`, time.Now().Unix()-1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("expired grant replay", err)
	}
	if _, err := f.db.Exec(`UPDATE order_guest_access SET expires_at=?`, time.Now().Unix()+3600); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RevokeAllGuestAccess(ctx, f.order); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, saleAction(c.Version, "withdraw", "")); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("revoked grant write", err)
	}
	if _, err := f.s.GuestAfterSales(ctx, f.order, f.token); !errors.Is(err, ErrGuestVerification) {
		t.Fatal("revoked grant read", err)
	}
	saleCount(t, f.db, 1, 1)
}

func TestAfterSaleConcurrentCreateReplayAndActiveLimit(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	input := saleInput("refund")
	f.db.SetMaxOpenConns(4)
	var wg sync.WaitGroup
	results := make(chan string, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := f.s.CreateAfterSale(ctx, f.order, f.token, input)
			results <- c.PublicID
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for got := range results {
		if id == "" {
			id = got
		}
		if got != id {
			t.Fatal("same-key produced different cases")
		}
	}
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, saleInput("refund")); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("second active request", err)
	}
	input.Reason = "Changed intent"
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("same-key different intent", err)
	}
	saleCount(t, f.db, 1, 1)
}

func TestAfterSaleReturnInspectionAndVersionedActions(t *testing.T) {
	f := saleFixture(t, true)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("return_refund"))
	approve := saleAction(1, "approve", "Return only to the address confirmed by support.\r\nUse the provided return label.")
	c = saleReview(t, f, c, approve)
	if c.State != "awaiting_return" || strings.Contains(c.ReturnInstructions, "\r") || c.Version != 2 {
		t.Fatalf("approval %+v", c)
	}
	if _, err := f.s.ReviewAfterSale(ctx, f.order, c.PublicID, saleAction(2, "approve", "Approve before inspection")); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("skipped return inspection", err)
	}
	tracking := saleAction(2, "return_tracking", "")
	tracking.Carrier = "Return carrier"
	tracking.TrackingNumber = "INBOUND-1"
	c, err := f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, tracking)
	if err != nil || c.State != "awaiting_return" || c.ReturnTrackingNumber != "INBOUND-1" || c.Version != 3 {
		t.Fatalf("tracking %+v %v", c, err)
	}
	if _, err = f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, saleAction(3, "withdraw", "")); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("withdraw after dispatched return", err)
	}
	if _, err = f.s.ReviewAfterSale(ctx, f.order, c.PublicID, saleAction(2, "receive_return", "Received")); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("stale page overwrote tracking", err)
	}
	c = saleReview(t, f, c, saleAction(3, "receive_return", "Parcel received; inspection is pending."))
	c = saleReview(t, f, c, saleAction(4, "approve", "Inspection passed. Refund approved for the requested amount."))
	if c.State != "approved" || c.Version != 5 || len(c.Events) != 5 || c.Events[2].TrackingNumber != "INBOUND-1" || !strings.Contains(c.ReturnInstructions, "return label") {
		t.Fatalf("history %+v", c)
	}
	if _, err = f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, saleAction(5, "approve", "Impersonation")); !errors.Is(err, ErrAfterSaleInvalid) {
		t.Fatal("customer approved own refund", err)
	}
	if _, err = f.s.ReviewAfterSale(ctx, "other-order", c.PublicID, saleAction(5, "reject", "Wrong object")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cross-object merchant update", err)
	}
	var status string
	var quantity, reserved, requests int
	if err = f.db.QueryRow(`SELECT status,(SELECT quantity FROM inventory),(SELECT reserved FROM inventory),(SELECT COUNT(*) FROM refund_request) FROM "order" WHERE public_id=?`, f.order).Scan(&status, &quantity, &reserved, &requests); err != nil || status != "fulfilled" || quantity != 4 || reserved != 0 || requests != 0 {
		t.Fatalf("review moved money/stock: %s %d %d %d %v", status, quantity, reserved, requests, err)
	}
	assertShipmentCount(t, f.db, 1, 1, 1, 1) // review does not invent an email delivery
}

func TestAfterSaleActionReplayAndConcurrentReview(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("refund"))
	f.db.SetMaxOpenConns(4)
	action := saleAction(1, "approve", "Approved for the requested amount.")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.ReviewAfterSale(ctx, f.order, c.PublicID, action); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	saleCount(t, f.db, 1, 2)
	changed := action
	changed.Message = "Different approval"
	if _, err := f.s.ReviewAfterSale(ctx, f.order, c.PublicID, changed); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("key reuse with changed message", err)
	}
	if _, err := f.s.ReviewAfterSale(ctx, f.order, c.PublicID, saleAction(1, "reject", "Other browser")); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("stale concurrent review", err)
	}
	if _, err := f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, action); !errors.Is(err, ErrAfterSaleInvalid) {
		t.Fatal("actor escalation", err)
	}
}

func TestAfterSaleConcurrentDifferentKeysOnlyOneActive(t *testing.T) {
	f := saleFixture(t, false)
	f.db.SetMaxOpenConns(4)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		input := saleInput("refund")
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.CreateAfterSale(ctx, f.order, f.token, input)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	winners := 0
	for err := range errs {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrAfterSaleChanged) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("active winners %d", winners)
	}
	saleCount(t, f.db, 1, 1)
}

func TestAfterSaleGuestTrackingBoundLeavesResolutionCapacity(t *testing.T) {
	f := saleFixture(t, true)
	ctx := context.Background()
	input := saleInput("return_refund")
	c := saleCreate(t, f, input)
	c = saleReview(t, f, c, saleAction(1, "approve", "Contact support for the correct return label."))
	for c.Version < AfterSaleEventLimit-3 {
		action := saleAction(c.Version, "return_tracking", "Corrected tracking details.")
		action.Carrier, action.TrackingNumber = "Return carrier", "INBOUND"
		var err error
		c, err = f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	action := saleAction(c.Version, "return_tracking", "")
	action.Carrier, action.TrackingNumber = "Return carrier", "ANOTHER"
	if _, err := f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, action); !errors.Is(err, ErrAfterSaleLimit) {
		t.Fatal("tracking exhausted merchant resolution", err)
	}
	c = saleReview(t, f, c, saleAction(c.Version, "receive_return", "Received and ready for inspection."))
	c = saleReview(t, f, c, saleAction(c.Version, "approve", "Inspection passed."))
	refund := saleRefund(t, f, c.AmountCents)
	complete := saleAction(c.Version, "complete", "Refund processed.")
	complete.RefundRequestID = refund
	c = saleReview(t, f, c, complete)
	if c.State != "completed" || c.Version != AfterSaleEventLimit || len(c.Events) != AfterSaleEventLimit {
		t.Fatal("reserved resolution steps did not fit")
	}
	// An old create retry still resolves the original completed case, even with
	// a lower remaining balance and changed policy/state.
	replayed, err := f.s.CreateAfterSale(ctx, f.order, f.token, input)
	if err != nil || replayed.PublicID != c.PublicID || replayed.State != "completed" || replayed.RefundRequestID != "" {
		t.Fatal("old create was not safely replayed", err)
	}
}

func TestAfterSaleClosedRequestLimitRemainsBounded(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	for range AfterSaleLimit {
		c := saleCreate(t, f, saleInput("refund"))
		saleReview(t, f, c, saleAction(c.Version, "reject", "Please contact support for further assistance."))
	}
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, saleInput("refund")); !errors.Is(err, ErrAfterSaleLimit) {
		t.Fatal("unbounded requests", err)
	}
	h, err := f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || len(h.Cases) != AfterSaleLimit || h.CanRequestRefund || h.CanRequestReturn {
		t.Fatalf("bounded snapshot %+v %v", h, err)
	}
}

func saleRefund(t *testing.T, f afterSaleFixture, amount int64) string {
	t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	_, err := f.db.Exec(`INSERT INTO refund_request(public_id,client_key,order_id,receipt_id,provider,payment_ref,amount_cents,currency,initial_refunded_cents,config_fingerprint,state,gateway_ref) SELECT ?,?,order_id,id,provider,payment_ref,?,currency,0,'fixture','succeeded',? FROM payment_receipt WHERE payment_ref='fixture-sale-payment'`, id, id, amount, "fixture-refund-"+id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`INSERT INTO payment_refund(receipt_id,provider,refund_ref,amount_cents) SELECT receipt_id,provider,gateway_ref,amount_cents FROM refund_request WHERE public_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE payment_receipt SET refunded_cents=refunded_cents+? WHERE payment_ref='fixture-sale-payment'`, amount); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAfterSaleCompletionRequiresVerifiedMatchingRefund(t *testing.T) {
	f := saleFixture(t, true)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("refund"))
	c = saleReview(t, f, c, saleAction(1, "approve", "Approved."))
	refund := saleRefund(t, f, c.AmountCents)
	complete := saleAction(2, "complete", "Refund was processed to the original method.")
	complete.RefundRequestID = refund
	for _, statement := range []string{
		`UPDATE refund_request SET state='pending'`,
		`UPDATE refund_request SET amount_cents=999`,
		`UPDATE refund_request SET currency='EUR'`,
		`UPDATE refund_request SET created_at='2000-01-01T00:00:00.000Z'`,
		`UPDATE payment_refund SET amount_cents=999`,
		`UPDATE payment_receipt SET refunded_cents=0`,
	} {
		tx, err := f.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
		// Commit the corrupt observation, verify the service refuses it, then
		// restore from the known fixture without ever submitting another refund.
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.ReviewAfterSale(ctx, f.order, c.PublicID, complete); !errors.Is(err, ErrAfterSaleChanged) {
			t.Fatalf("unverified completion accepted (%s): %v", statement, err)
		}
		for _, restore := range []string{
			`UPDATE refund_request SET state='succeeded',amount_cents=1000,currency='USD',created_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
			`UPDATE payment_refund SET amount_cents=1000`,
			`UPDATE payment_receipt SET refunded_cents=1000`,
		} {
			if _, err = f.db.Exec(restore); err != nil {
				t.Fatal(err)
			}
		}
	}
	c = saleReview(t, f, c, complete)
	if c.State != "completed" || c.RefundRequestID != refund || c.RefundNeedsReview || c.Version != 3 {
		t.Fatalf("complete %+v", c)
	}
	c = saleReview(t, f, c, complete)
	saleCount(t, f.db, 1, 3)
	h, err := f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || h.Cases[0].RefundRequestID != "" || h.MaxRequestCents != 2000 || !h.CanRequestRefund {
		t.Fatalf("guest completed view %+v %v", h, err)
	}
	// A later inconsistent observation must be visible; never erase the recorded
	// review or pretend that an old success settles a new discrepancy.
	if _, err = f.db.Exec(`UPDATE refund_request SET state='review'`); err != nil {
		t.Fatal(err)
	}
	h, err = f.s.GuestAfterSales(ctx, f.order, f.token)
	if err != nil || !h.Cases[0].RefundNeedsReview || h.Cases[0].State != "completed" {
		t.Fatal("later discrepancy hidden", err)
	}
	assertShipmentCount(t, f.db, 1, 1, 1, 1)
}

func TestAfterSaleCompletionRejectsOpenReconciliationAndReuse(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("refund"))
	c = saleReview(t, f, c, saleAction(1, "approve", "Approved."))
	refund := saleRefund(t, f, c.AmountCents)
	complete := saleAction(2, "complete", "Refund processed.")
	complete.RefundRequestID = refund
	_, err := f.db.Exec(`INSERT INTO refund_reconciliation(public_id,receipt_id,provider,refund_ref,amount_cents,observed_state)
SELECT ?,receipt_id,provider,gateway_ref,amount_cents,'failed' FROM refund_request WHERE public_id=?`, uuid.Must(uuid.NewV7()).String(), refund)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReviewAfterSale(ctx, f.order, c.PublicID, complete); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("open money discrepancy completed case", err)
	}
	if _, err = f.db.Exec(`UPDATE refund_reconciliation SET state='resolved'`); err != nil {
		t.Fatal(err)
	}
	c = saleReview(t, f, c, complete)
	second := saleCreate(t, f, saleInput("refund"))
	second = saleReview(t, f, second, saleAction(1, "approve", "Second request approved."))
	// Even if a clock boundary or imported timestamp would make the old refund
	// appear recent, its ledger evidence may settle only the first case.
	if _, err = f.db.Exec(`UPDATE refund_request SET created_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`); err != nil {
		t.Fatal(err)
	}
	complete = saleAction(2, "complete", "Attempt reuse.")
	complete.RefundRequestID = refund
	if _, err = f.s.ReviewAfterSale(ctx, f.order, second.PublicID, complete); !errors.Is(err, ErrAfterSaleChanged) {
		t.Fatal("same refund settled two cases", err)
	}
	h, err := f.s.AdminAfterSales(ctx, f.order)
	if err != nil || len(h.Cases) != 2 || h.Cases[0].State != "approved" || h.Cases[1].State != "completed" {
		t.Fatal("rejected reuse changed history", err)
	}
}

func TestAfterSaleExistingOtherOrderCannotReadOrActOnCase(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("refund"))
	other := "existing-other-order"
	_, err := f.db.Exec(`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) SELECT ?,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents FROM "order" WHERE public_id=?`, other, f.order)
	if err != nil {
		t.Fatal(err)
	}
	token := guestBrowser(t)
	_, err = f.db.Exec(`INSERT INTO order_guest_access(order_id,token_hash,expires_at) SELECT id,?,? FROM "order" WHERE public_id=?`, guestTokenHash(token), time.Now().Unix()+3600, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.UpdateGuestAfterSale(ctx, other, c.PublicID, token, saleAction(1, "withdraw", "")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("other valid grant acted on case", err)
	}
	if _, err = f.s.ReviewAfterSale(ctx, other, c.PublicID, saleAction(1, "reject", "Wrong order")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("merchant cross-object write", err)
	}
	h, err := f.s.GuestAfterSales(ctx, other, token)
	if err != nil || len(h.Cases) != 0 {
		t.Fatal("other private order leaked case", err)
	}
	saleCount(t, f.db, 1, 1)
}

func TestAfterSaleBlocksFirstShipmentAndWorkbench(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	c := saleCreate(t, f, saleInput("refund"))
	bench, err := f.s.AdminSearch(ctx, OrderSearchFilter{View: "ready_to_fulfill"}, true)
	if err != nil || bench.Total != 0 || bench.Counts["refunds_pending"] != 1 {
		t.Fatalf("workbench %+v %v", bench, err)
	}
	if err = f.s.Fulfill(ctx, f.order, "Carrier", "OUT-1", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("active request allowed shipment", err)
	}
	assertShipmentCount(t, f.db, 0, 0, 0, 0)
	withdraw := saleAction(1, "withdraw", "")
	c, err = f.s.UpdateGuestAfterSale(ctx, f.order, c.PublicID, f.token, withdraw)
	if err != nil || c.State != "withdrawn" {
		t.Fatal("withdraw", err)
	}
	bench, err = f.s.AdminSearch(ctx, OrderSearchFilter{View: "ready_to_fulfill"}, true)
	if err != nil || bench.Total != 1 || bench.Counts["refunds_pending"] != 0 {
		t.Fatalf("unblocked workbench %+v %v", bench, err)
	}
	if err = f.s.Fulfill(ctx, f.order, "Carrier", "OUT-1", ""); err != nil {
		t.Fatal(err)
	}
	assertShipmentCount(t, f.db, 1, 1, 1, 1)
}

func TestAfterSaleEventFailureRollsBackRequestAndAction(t *testing.T) {
	f := saleFixture(t, false)
	ctx := context.Background()
	input := saleInput("refund")
	if _, err := f.db.Exec(`CREATE TRIGGER reject_sale_event BEFORE INSERT ON after_sale_event BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateAfterSale(ctx, f.order, f.token, input); err == nil {
		t.Fatal("event failure ignored")
	}
	saleCount(t, f.db, 0, 0)
	if _, err := f.db.Exec(`DROP TRIGGER reject_sale_event`); err != nil {
		t.Fatal(err)
	}
	c := saleCreate(t, f, input)
	if _, err := f.db.Exec(`CREATE TRIGGER reject_sale_event BEFORE INSERT ON after_sale_event BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReviewAfterSale(ctx, f.order, c.PublicID, saleAction(1, "approve", "Approved")); err == nil {
		t.Fatal("review event failure ignored")
	}
	h, err := f.s.AdminAfterSales(ctx, f.order)
	if err != nil || h.Cases[0].Version != 1 || h.Cases[0].State != "requested" || h.Cases[0].MerchantMessage != "" {
		t.Fatalf("partial review committed %+v %v", h, err)
	}
	saleCount(t, f.db, 1, 1)
	assertShipmentCount(t, f.db, 0, 0, 0, 0)
}

func TestAfterSaleMigrationNoBackfillAndRestart(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/upgrade.db"
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
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
		if file.Name() >= "0038" {
			pending++
			continue
		}
		b, err := fs.ReadFile(migrations.FS, file.Name())
		if err != nil {
			t.Fatal(err)
		}
		old[file.Name()] = &fstest.MapFile{Data: b}
	}
	if _, err = migrate.Run(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO customer(id,public_id,email) VALUES(1,'legacy-sale-customer','legacy@example.invalid')`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(1,'legacy-sale-order',1,'fulfilled','legacy@example.invalid','Original','Original address',100,100)`,
		`INSERT INTO shipment(order_id,carrier,tracking_number) VALUES(1,'Original carrier','ORIGINAL-1')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := migrate.Run(ctx, db, migrations.FS); err != nil || n != pending {
		t.Fatalf("applied %d %v", n, err)
	}
	saleCount(t, db, 0, 0)
	assertShipmentCount(t, db, 1, 0, 0, 0)
	token := guestBrowser(t)
	if _, err = db.Exec(`INSERT INTO order_guest_access(order_id,token_hash,expires_at) VALUES(1,?,?)`, guestTokenHash(token), time.Now().Unix()+3600); err != nil {
		t.Fatal(err)
	}
	s := New(db, settings.New(db))
	input := saleInput("refund")
	input.AmountCents = 50
	c, err := s.CreateAfterSale(ctx, "legacy-sale-order", token, input)
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.ReviewAfterSale(ctx, "legacy-sale-order", c.PublicID, saleAction(1, "approve", "Approved; actual refund still requires verified payment evidence."))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if n, err := migrate.Run(ctx, db, migrations.FS); err != nil || n != 0 {
		t.Fatalf("re-entry %d %v", n, err)
	}
	s = New(db, settings.New(db))
	o, err := s.Get(ctx, "legacy-sale-order")
	if err != nil || o.ShipAddress != "Original address" || o.TrackingNumber != "ORIGINAL-1" || o.Status != "fulfilled" {
		t.Fatal("legacy facts changed", err)
	}
	h, err := s.AdminAfterSales(ctx, "legacy-sale-order")
	if err != nil || len(h.Cases) != 1 || h.Cases[0].PublicID != c.PublicID || h.Cases[0].State != "approved" || len(h.Cases[0].Events) != 2 || h.CanRequestRefund || h.CanRequestReturn {
		t.Fatalf("legacy snapshot %+v %v", h, err)
	}
	replayed, err := s.CreateAfterSale(ctx, "legacy-sale-order", token, input)
	if err != nil || replayed.PublicID != c.PublicID || replayed.State != "approved" {
		t.Fatal("lost create response duplicated case after real database reopen", err)
	}
	saleCount(t, db, 1, 2)
	assertShipmentCount(t, db, 1, 0, 0, 0)
}
