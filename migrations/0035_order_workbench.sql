-- 订单工作台索引 / Order Workbench Indexes
-- 功能：支持按创建时间分页和订单未决退款筛选，不改订单或资金记录
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-25 13:42:16
CREATE INDEX IF NOT EXISTS ix_order_admin_created ON "order"(created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ix_refund_request_order_active ON refund_request(order_id)
WHERE state IN ('prepared','pending','review');
