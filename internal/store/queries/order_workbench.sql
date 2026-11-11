-- Order Workbench / Dingdan Gongzuotai
-- Purpose: bounded private search, stable pages and consistent operational views
-- Author: daxing Email: 3442535897@qq.com Time: 2026-10-25 13:42:16

-- name: CountAdminOrderViews :one
WITH scoped AS (
 SELECT o.id,o.public_id,o.status,
  CAST(CASE WHEN CAST(sqlc.arg(customer_data) AS INTEGER)=1 THEN o.email ELSE '' END AS TEXT) AS email,
  o.currency,o.total_cents,o.payment_provider,o.created_at,
  CAST(EXISTS(SELECT 1 FROM shipment s WHERE s.order_id=o.id) AS INTEGER) AS shipped,
  CAST(EXISTS(SELECT 1 FROM refund_request rr WHERE rr.order_id=o.id AND rr.state IN ('prepared','pending','review'))
   OR EXISTS(SELECT 1 FROM after_sale a WHERE a.order_id=o.id AND a.state IN ('requested','awaiting_return','returned','approved'))
   OR EXISTS(SELECT 1 FROM refund_reconciliation rc JOIN payment_receipt p ON p.id=rc.receipt_id WHERE p.order_id=o.id AND rc.state='open') AS INTEGER) AS refund_pending,
  CAST(o.status='payment_review' OR EXISTS(
   SELECT 1 FROM payment_receipt p WHERE p.order_id=o.id AND (
    (p.review_reason<>'' AND p.refunded_cents<p.amount_cents) OR
    EXISTS(SELECT 1 FROM refund_reconciliation rc WHERE rc.receipt_id=p.id AND rc.state='open')
   )) AS INTEGER) AS needs_review
 FROM "order" o
 WHERE (CAST(sqlc.arg(status) AS TEXT)='' OR o.status=sqlc.arg(status))
  AND (CAST(sqlc.arg(provider) AS TEXT)='' OR o.payment_provider=sqlc.arg(provider))
  AND (CAST(sqlc.arg(from_time) AS TEXT)='' OR o.created_at>=sqlc.arg(from_time))
  AND (CAST(sqlc.arg(to_time) AS TEXT)='' OR o.created_at<sqlc.arg(to_time))
  AND (CAST(sqlc.arg(term) AS TEXT)='' OR instr(lower(o.public_id),sqlc.arg(term))>0
   OR (CAST(sqlc.arg(customer_data) AS INTEGER)=1 AND instr(lower(o.email),sqlc.arg(term))>0))
), classified AS (
 SELECT *,CAST(status='paid' AND shipped=0 AND refund_pending=0 AND needs_review=0 AS INTEGER) AS ready_to_fulfill
 FROM scoped
)
SELECT COUNT(*) AS all_orders,
 CAST(COALESCE(SUM(status='pending'),0) AS INTEGER) AS awaiting_payment,
 CAST(COALESCE(SUM(ready_to_fulfill),0) AS INTEGER) AS ready_to_fulfill,
 CAST(COALESCE(SUM(needs_review),0) AS INTEGER) AS needs_review,
 CAST(COALESCE(SUM(refund_pending),0) AS INTEGER) AS refunds_pending,
 CAST(COALESCE(SUM(shipped),0) AS INTEGER) AS shipped
FROM classified;

-- name: ListAdminOrderPage :many
WITH scoped AS (
 SELECT o.id,o.public_id,o.status,
  CAST(CASE WHEN CAST(sqlc.arg(customer_data) AS INTEGER)=1 THEN o.email ELSE '' END AS TEXT) AS email,
  o.currency,o.total_cents,o.payment_provider,o.created_at,
  CAST(EXISTS(SELECT 1 FROM shipment s WHERE s.order_id=o.id) AS INTEGER) AS shipped,
  CAST(EXISTS(SELECT 1 FROM refund_request rr WHERE rr.order_id=o.id AND rr.state IN ('prepared','pending','review'))
   OR EXISTS(SELECT 1 FROM after_sale a WHERE a.order_id=o.id AND a.state IN ('requested','awaiting_return','returned','approved'))
   OR EXISTS(SELECT 1 FROM refund_reconciliation rc JOIN payment_receipt p ON p.id=rc.receipt_id WHERE p.order_id=o.id AND rc.state='open') AS INTEGER) AS refund_pending,
  CAST(o.status='payment_review' OR EXISTS(
   SELECT 1 FROM payment_receipt p WHERE p.order_id=o.id AND (
    (p.review_reason<>'' AND p.refunded_cents<p.amount_cents) OR
    EXISTS(SELECT 1 FROM refund_reconciliation rc WHERE rc.receipt_id=p.id AND rc.state='open')
   )) AS INTEGER) AS needs_review
 FROM "order" o
 WHERE (CAST(sqlc.arg(status) AS TEXT)='' OR o.status=sqlc.arg(status))
  AND (CAST(sqlc.arg(provider) AS TEXT)='' OR o.payment_provider=sqlc.arg(provider))
  AND (CAST(sqlc.arg(from_time) AS TEXT)='' OR o.created_at>=sqlc.arg(from_time))
  AND (CAST(sqlc.arg(to_time) AS TEXT)='' OR o.created_at<sqlc.arg(to_time))
  AND (CAST(sqlc.arg(term) AS TEXT)='' OR instr(lower(o.public_id),sqlc.arg(term))>0
   OR (CAST(sqlc.arg(customer_data) AS INTEGER)=1 AND instr(lower(o.email),sqlc.arg(term))>0))
), classified AS (
 SELECT *,CAST(status='paid' AND shipped=0 AND refund_pending=0 AND needs_review=0 AS INTEGER) AS ready_to_fulfill
 FROM scoped
)
SELECT public_id,status,email,currency,total_cents,payment_provider,created_at,shipped,refund_pending,needs_review,ready_to_fulfill
FROM classified
WHERE CAST(sqlc.arg(view) AS TEXT)='all'
 OR (sqlc.arg(view)='awaiting_payment' AND status='pending')
 OR (sqlc.arg(view)='ready_to_fulfill' AND ready_to_fulfill=1)
 OR (sqlc.arg(view)='needs_review' AND needs_review=1)
 OR (sqlc.arg(view)='refunds_pending' AND refund_pending=1)
 OR (sqlc.arg(view)='shipped' AND shipped=1)
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);
