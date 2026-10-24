-- 收款账本 / Payment Receipt Ledger
-- 功能：独立记录每笔到账及已退金额，保留迟到和重复收款以供核对；不改写发货记录
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 16:30:00
-- Refuse ambiguous legacy payment references instead of attaching money to an
-- arbitrary order. The upgrade transaction and its pre-upgrade backup are kept.
CREATE UNIQUE INDEX IF NOT EXISTS ux_order_primary_payment ON "order"(payment_provider,payment_ref)
WHERE payment_ref<>'' AND payment_provider IN ('stripe','paypal');
CREATE TABLE IF NOT EXISTS payment_receipt (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id INTEGER NOT NULL REFERENCES "order"(id),
    provider TEXT NOT NULL CHECK(provider IN ('stripe','paypal')),
    payment_ref TEXT NOT NULL CHECK(payment_ref<>''),
    amount_cents INTEGER NOT NULL CHECK(amount_cents>0),
    currency TEXT NOT NULL,
    refunded_cents INTEGER NOT NULL DEFAULT 0 CHECK(refunded_cents>=0 AND refunded_cents<=amount_cents),
    review_reason TEXT NOT NULL DEFAULT '' CHECK(review_reason IN ('','late_payment','duplicate_payment','legacy_refund')),
    source TEXT NOT NULL DEFAULT 'verified' CHECK(source IN ('legacy','verified')),
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(provider,payment_ref)
);
CREATE INDEX IF NOT EXISTS ix_payment_receipt_order ON payment_receipt(order_id);
-- Legacy refunded status alone is not evidence of a full refund: old callbacks
-- also marked partial refunds as full. Import only recorded refund amounts and
-- flag such orders for reconciliation with the provider.
INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency,refunded_cents,review_reason,source,created_at)
SELECT o.id,o.payment_provider,o.payment_ref,o.total_cents,UPPER(o.currency),
    MIN(o.total_cents,COALESCE((SELECT SUM(r.amount_cents) FROM refund r WHERE r.order_id=o.id AND r.provider=o.payment_provider),0)),
    CASE WHEN o.status='payment_review' THEN 'late_payment'
         WHEN o.status='refunded' THEN 'legacy_refund' ELSE '' END,
    'legacy',o.created_at
FROM "order" o WHERE o.payment_ref<>'' AND o.payment_provider IN ('stripe','paypal') AND o.total_cents>0
ON CONFLICT(provider,payment_ref) DO NOTHING;

CREATE TABLE IF NOT EXISTS payment_refund (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    receipt_id INTEGER NOT NULL REFERENCES payment_receipt(id),
    provider TEXT NOT NULL,
    refund_ref TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK(amount_cents>0),
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(provider,refund_ref)
);
INSERT INTO payment_refund(receipt_id,provider,refund_ref,amount_cents,created_at)
SELECT p.id,r.provider,r.provider_refund_id,r.amount_cents,r.created_at
FROM refund r JOIN payment_receipt p ON p.order_id=r.order_id AND p.provider=r.provider
WHERE r.amount_cents>0
ON CONFLICT(provider,refund_ref) DO NOTHING;
