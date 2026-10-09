-- 订单库存预留期限 / Order Reservation Expiry
-- 功能：未付款订单 30 分钟期限；将历史已售预留转成实际库存扣减并保持当前可售数量
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-09 11:24:00

ALTER TABLE "order" ADD COLUMN reservation_expires_at TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS ix_order_reservation_expiry ON "order"(status, reservation_expires_at);

UPDATE "order"
SET reservation_expires_at = strftime('%Y-%m-%dT%H:%M:%fZ', created_at, '+30 minutes')
WHERE status = 'pending' AND reservation_expires_at = '';

-- 旧版 reserved 同时包含未付和已售数量；先保留原本的可售量，再只保留未付预留。
UPDATE inventory
SET quantity = max(quantity - reserved, 0) + COALESCE((
        SELECT SUM(oi.quantity) FROM order_item oi
        JOIN "order" o ON o.id = oi.order_id
        WHERE oi.variant_id = inventory.variant_id AND o.status = 'pending'
    ), 0),
    reserved = COALESCE((
        SELECT SUM(oi.quantity) FROM order_item oi
        JOIN "order" o ON o.id = oi.order_id
        WHERE oi.variant_id = inventory.variant_id AND o.status = 'pending'
    ), 0);
