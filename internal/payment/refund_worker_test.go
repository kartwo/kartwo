// 退款查询恢复测试 / Refund Reconciliation Worker Tests
// 功能：验证只读恢复、查询间隔、锁定与配置变更、过期未知结果及并行工作器去重
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 23:50:00
package payment

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"
)

func makeRefundCheckDue(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`UPDATE refund_request SET updated_at='2000-01-01T00:00:00.000Z'`); err != nil {
		t.Fatal(err)
	}
}

func TestRefundWorkerKnownResultAndConcurrentPolls(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "worker", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.initialState = "pending"
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "worker", refundInput("worker-request-0001", 500, 0))
	if err != nil || r.State != "pending" {
		t.Fatalf("pending: %+v %v", r, err)
	}
	// Fresh requests must not immediately trigger another provider call.
	if err := svc.pollRefunds(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	lookups := f.lookups
	f.mu.Unlock()
	if lookups != 0 {
		t.Fatal("ignored retry delay")
	}
	f.state(r.ID, "succeeded")
	makeRefundCheckDue(t, db)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			// Separate service objects model competing processes sharing the DB.
			worker := NewService(db, svc.settings, svc.keys)
			worker.stripe.apiBase = ts.URL
			if err := worker.pollRefunds(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	assertLedger(t, db, 2500, 500, 1)
	var state string
	if err := db.QueryRow(`SELECT state FROM refund_request`).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("worker did not finish: %s %v", state, err)
	}
	if posts, creates := f.counts(); posts != 1 || creates != 1 {
		t.Fatalf("worker sent a money-moving request: %d %d", posts, creates)
	}
	f.mu.Lock()
	lookups = f.lookups
	f.mu.Unlock()
	if lookups != 1 {
		t.Fatalf("concurrent polls made %d lookups", lookups)
	}
}

func TestRefundWorkerUnknownResultNeverPosts(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "worker-unknown", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.loseFirst = true
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "worker-unknown", refundInput("worker-unknown-0001", 500, 0))
	if err != nil || r.State != "pending" {
		t.Fatalf("unknown: %+v %v", r, err)
	}
	makeRefundCheckDue(t, db)
	if err := svc.pollRefunds(context.Background()); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM refund_request`).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("recent unknown should wait: %s %v", state, err)
	}
	if _, err := db.Exec(`UPDATE refund_request SET first_sent_at=?`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	makeRefundCheckDue(t, db)
	if err := svc.pollRefunds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state FROM refund_request`).Scan(&state); err != nil || state != "review" {
		t.Fatalf("expired unknown should require review: %s %v", state, err)
	}
	if posts, _ := f.counts(); posts != 1 {
		t.Fatalf("unknown refund resubmitted %d times", posts)
	}
	assertLedger(t, db, 2500, 0, 1)
}

func TestRefundWorkerLockedCredentialsAndFailedResult(t *testing.T) {
	svc, db, kek := newTestSvc(t)
	seedPaidOrder(t, db, "worker-locked", 2500, "pi-refund")
	f, ts := newRefundServer(t)
	f.initialState = "pending"
	svc.stripe.apiBase = ts.URL
	r, err := svc.RequestRefund(context.Background(), "worker-locked", refundInput("worker-locked-0001", 500, 0))
	if err != nil {
		t.Fatal(err)
	}
	svc.keys.Lock()
	makeRefundCheckDue(t, db)
	if err := svc.pollRefunds(context.Background()); err != nil {
		t.Fatal(err)
	}
	var reason, state, updated string
	if err := db.QueryRow(`SELECT state,reason,updated_at FROM refund_request`).Scan(&state, &reason, &updated); err != nil || state != "pending" || reason != "credentials_locked" || updated == "2000-01-01T00:00:00.000Z" {
		t.Fatalf("locked credentials: %s %s %s %v", state, reason, updated, err)
	}
	if err := svc.keys.Unlock(context.Background(), kek); err != nil {
		t.Fatal(err)
	}
	f.state(r.ID, "failed")
	makeRefundCheckDue(t, db)
	if err := svc.pollRefunds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state FROM refund_request`).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("failed result not synchronized: %s %v", state, err)
	}
	assertLedger(t, db, 2500, 0, 1)
	if posts, _ := f.counts(); posts != 1 {
		t.Fatal("worker resubmitted failed refund")
	}
}

func TestRefundWorkerBoundedBatchDoesNotStarveLockedRequests(t *testing.T) {
	svc, db, _ := newTestSvc(t)
	seedPaidOrder(t, db, "worker-batch", 2500, "pi-original")
	for i := range 12 {
		ref, id := fmt.Sprintf("pi-batch-%d", i), fmt.Sprintf("batch-request-%d", i)
		if _, err := db.Exec(`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency) SELECT id,'stripe',?,2500,'USD' FROM "order"`, ref); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO refund_request(public_id,client_key,order_id,receipt_id,provider,payment_ref,amount_cents,currency,initial_refunded_cents,config_fingerprint,state,gateway_ref)
            SELECT ?,?,order_id,id,provider,payment_ref,100,currency,0,'original-config','pending',? FROM payment_receipt WHERE payment_ref=?`, id, id, "re-"+id, ref); err != nil {
			t.Fatal(err)
		}
	}
	svc.keys.Lock()
	makeRefundCheckDue(t, db)
	for pass, want := range []int{2, 0} {
		if err := svc.pollRefunds(context.Background()); err != nil {
			t.Fatal(err)
		}
		var remaining int
		if err := db.QueryRow(`SELECT COUNT(*) FROM refund_request WHERE updated_at='2000-01-01T00:00:00.000Z'`).Scan(&remaining); err != nil || remaining != want {
			t.Fatalf("pass %d: %d unvisited, want %d: %v", pass, remaining, want, err)
		}
	}
}
