// 订单工作台测试 / Order Workbench Tests
// 功能：验证 SQL 分页/视图、权限搜索隔离、日期边界与无效条件，覆盖退款后履约记录
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16
package order

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"
)

func seedWorkbench(t *testing.T) *Service {
	t.Helper()
	db, service, _, _ := setup(t)
	if _, err := db.Exec(`INSERT INTO customer(id,public_id,email) VALUES(801,'bench-customer','bench-private@example.invalid')`); err != nil {
		t.Fatal(err)
	}
	states := []string{"pending", "paid", "paid", "paid", "fulfilled", "refunded", "payment_review", "paid", "cancelled", "pending", "paid"}
	for index, state := range states {
		id, created, provider := index+1, "2026-10-01T12:00:00.000Z", "stripe"
		if id == 10 {
			created, provider = "2026-10-02T00:00:00.000Z", "paypal"
		}
		if id == 11 {
			created = "2026-10-01T23:59:59.999Z"
		}
		if _, err := db.Exec(`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents,payment_provider,created_at) VALUES(?,?,801,?,'bench-private@example.invalid','Fixture','Fixture','USD',100,100,?,?)`, id, fmt.Sprintf("bench-order-%02d", id), state, provider, created); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO payment_receipt(id,order_id,provider,payment_ref,amount_cents,currency) VALUES(3,3,'stripe','bench-payment-3',100,'USD'),(4,4,'stripe','bench-payment-4',100,'USD'),(7,7,'stripe','bench-payment-7',100,'USD'),(8,8,'stripe','bench-payment-8',100,'USD')`,
		`INSERT INTO refund_request(public_id,client_key,order_id,receipt_id,provider,payment_ref,amount_cents,currency,initial_refunded_cents,config_fingerprint,state) VALUES('bench-refund-3','key3',3,3,'stripe','bench-payment-3',10,'USD',0,'fixture','prepared'),('bench-refund-8','key8',8,8,'stripe','bench-payment-8',10,'USD',0,'fixture','failed')`,
		`INSERT INTO refund_reconciliation(public_id,receipt_id,provider,refund_ref,amount_cents,observed_state) VALUES('bench-issue',4,'stripe','bench-refund-external',10,'pending')`,
		`UPDATE payment_receipt SET review_reason='duplicate_payment' WHERE id=7`,
		`INSERT INTO shipment(order_id,carrier,tracking_number) VALUES(5,'Fixture','TEST-5'),(6,'Fixture','TEST-6'),(11,'Fixture','TEST-11')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return service
}

func TestWorkbenchViewsRespectIndependentFinancialAndShipmentRecords(t *testing.T) {
	s := seedWorkbench(t)
	ctx := context.Background()
	wantCounts := map[string]int64{"all": 11, "awaiting_payment": 2, "ready_to_fulfill": 2, "needs_review": 2, "refunds_pending": 2, "shipped": 3}
	views := map[string][]string{"all": {"bench-order-10", "bench-order-11", "bench-order-09", "bench-order-08", "bench-order-07", "bench-order-06", "bench-order-05", "bench-order-04", "bench-order-03", "bench-order-02", "bench-order-01"},
		"awaiting_payment": {"bench-order-10", "bench-order-01"}, "ready_to_fulfill": {"bench-order-08", "bench-order-02"}, "needs_review": {"bench-order-07", "bench-order-04"}, "refunds_pending": {"bench-order-04", "bench-order-03"}, "shipped": {"bench-order-11", "bench-order-06", "bench-order-05"}}
	for view, expected := range views {
		t.Run(view, func(t *testing.T) {
			got, err := s.AdminSearch(ctx, OrderSearchFilter{View: view}, true)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, row := range got.Orders {
				ids = append(ids, row.PublicID)
				if row.ReadyToFulfill && (row.Shipped || row.RefundPending || row.NeedsReview || row.Status != "paid") {
					t.Fatal("blocked order offered for fulfillment")
				}
			}
			if !reflect.DeepEqual(ids, expected) || !reflect.DeepEqual(got.Counts, wantCounts) || got.Total != int64(len(expected)) {
				t.Fatalf("view=%s ids=%v counts=%v total=%d", view, ids, got.Counts, got.Total)
			}
		})
	}
	// Resolved external issues and terminal failed requests cease blocking; shipment history remains independent.
	if _, err := s.db.Exec(`UPDATE refund_reconciliation SET state='resolved'`); err != nil {
		t.Fatal(err)
	}
	got, err := s.AdminSearch(ctx, OrderSearchFilter{View: "ready_to_fulfill"}, false)
	if err != nil || got.Total != 3 || got.Counts["refunds_pending"] != 1 {
		t.Fatalf("resolved issue still blocks: %+v %v", got, err)
	}
}

func TestWorkbenchStableBoundedPaginationAndClamping(t *testing.T) {
	s := seedWorkbench(t)
	seen := map[string]bool{}
	for page := int64(1); page <= 6; page++ {
		got, err := s.AdminSearch(context.Background(), OrderSearchFilter{Page: page, PageSize: 2}, false)
		if err != nil || got.Page != page || got.Total != 11 || len(got.Orders) > 2 {
			t.Fatalf("page=%d result=%+v err=%v", page, got, err)
		}
		for _, row := range got.Orders {
			if seen[row.PublicID] || row.Email != "已隐藏" {
				t.Fatalf("duplicate or private row: %+v", row)
			}
			seen[row.PublicID] = true
		}
	}
	if len(seen) != 11 {
		t.Fatal("pagination omitted orders")
	}
	got, err := s.AdminSearch(context.Background(), OrderSearchFilter{Page: math.MaxInt64, PageSize: 2}, false)
	if err != nil || got.Page != 6 || len(got.Orders) != 1 {
		t.Fatalf("overflow-safe last page clamp: %+v %v", got, err)
	}
	empty, err := s.AdminSearch(context.Background(), OrderSearchFilter{Term: "no-match", Page: 99}, false)
	if err != nil || empty.Total != 0 || empty.Page != 1 || empty.Orders == nil || len(empty.Orders) != 0 {
		t.Fatalf("empty result: %+v %v", empty, err)
	}
}

func TestWorkbenchSearchPermissionLiteralTermsAndUTCDateBounds(t *testing.T) {
	s := seedWorkbench(t)
	for _, customers := range []bool{false, true} {
		got, err := s.AdminSearch(context.Background(), OrderSearchFilter{Term: " BENCH-PRIVATE@EXAMPLE.INVALID "}, customers)
		want := int64(0)
		if customers {
			want = 11
		}
		if err != nil || got.Total != want || got.Counts["all"] != want {
			t.Fatalf("email inference customers=%v got=%+v %v", customers, got, err)
		}
	}
	for _, term := range []string{"%", "_", "' OR 1=1 --"} {
		got, err := s.AdminSearch(context.Background(), OrderSearchFilter{Term: term}, true)
		if err != nil || got.Total != 0 {
			t.Fatalf("term must be literal: %q %+v %v", term, got, err)
		}
	}
	for _, test := range []struct {
		filter OrderSearchFilter
		want   int64
	}{
		{OrderSearchFilter{Term: "BENCH-ORDER-01"}, 1},
		{OrderSearchFilter{From: "2026-10-01", To: "2026-10-01"}, 10},
		{OrderSearchFilter{From: "2026-10-02", To: "2026-10-02"}, 1},
		{OrderSearchFilter{Status: "pending", Provider: "stripe"}, 1},
		{OrderSearchFilter{To: "9999-12-31"}, 11},
	} {
		got, err := s.AdminSearch(context.Background(), test.filter, true)
		if err != nil || got.Total != test.want {
			t.Fatalf("filter=%+v result=%+v err=%v", test.filter, got, err)
		}
	}
}

func TestWorkbenchRejectsInvalidInputsAndPreservesMigrationReentry(t *testing.T) {
	s := seedWorkbench(t)
	for _, filter := range []OrderSearchFilter{{Term: strings.Repeat("a", 161)}, {Term: "a\x00b"}, {View: "bogus"}, {Status: "bogus"}, {Provider: "bogus"}, {From: "2026-02-30"}, {To: "2026-1-1"}, {From: "2026-10-02", To: "2026-10-01"}, {Page: -1}, {PageSize: -1}, {PageSize: 101}} {
		if _, err := s.AdminSearch(context.Background(), filter, true); !errors.Is(err, ErrInvalidOrderSearch) {
			t.Fatalf("invalid filter accepted: %+v %v", filter, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.AdminSearch(ctx, OrderSearchFilter{}, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query became empty success: %v", err)
	}
	if count, err := migrate.Run(context.Background(), s.db, migrations.FS); err != nil || count != 0 {
		t.Fatalf("migration reentry: %d %v", count, err)
	}
}

func TestWorkbenchLargeResultsRemainBoundedAndShrinkToValidPage(t *testing.T) {
	s := seedWorkbench(t)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for id := 1000; id < 1240; id++ {
		if _, err := tx.Exec(`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(?,?,801,'paid','fixture@example.invalid','Fixture','Fixture',100,100)`, id, fmt.Sprintf("bulk-order-%d", id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{0, 20, 50, 100} {
		got, err := s.AdminSearch(context.Background(), OrderSearchFilter{PageSize: size}, false)
		want := size
		if want == 0 {
			want = 20
		}
		if err != nil || got.Total != 251 || int64(len(got.Orders)) != want {
			t.Fatalf("unbounded query: size=%d result=%+v err=%v", size, got, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE "order" SET status='cancelled' WHERE id>=1000`); err != nil {
		t.Fatal(err)
	}
	got, err := s.AdminSearch(context.Background(), OrderSearchFilter{View: "ready_to_fulfill", Page: 12}, false)
	if err != nil || got.Total != 2 || got.Page != 1 || len(got.Orders) != 2 {
		t.Fatalf("shrinking view must clamp: %+v %v", got, err)
	}
}
