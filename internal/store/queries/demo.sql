-- Public Demo Queries
-- Purpose: temporary product ownership, quota checks, and expiry cleanup
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-09-13 11:00:00

-- name: CountDemoProductsBySession :one
SELECT COUNT(*) FROM demo_product dp JOIN product p ON p.id = dp.product_id
WHERE dp.session_token = ? AND p.deleted_at IS NULL;

-- name: ListProductsVisibleToDemo :many
SELECT p.id, p.public_id, p.title, p.slug, p.status, p.featured, p.created_at, p.updated_at,
       EXISTS(SELECT 1 FROM demo_product own WHERE own.product_id = p.id AND own.session_token = ?) AS demo_owned
FROM product p
WHERE p.deleted_at IS NULL
  AND (NOT EXISTS(SELECT 1 FROM demo_product any_demo WHERE any_demo.product_id = p.id)
       OR EXISTS(SELECT 1 FROM demo_product own WHERE own.product_id = p.id AND own.session_token = ?))
ORDER BY p.id DESC;

-- name: CanDemoViewProduct :one
SELECT EXISTS(
    SELECT 1 FROM product p
    WHERE p.public_id = ? AND p.deleted_at IS NULL
      AND (NOT EXISTS(SELECT 1 FROM demo_product any_demo WHERE any_demo.product_id = p.id)
           OR EXISTS(SELECT 1 FROM demo_product own WHERE own.product_id = p.id AND own.session_token = ?))
);

-- name: ClaimDemoProduct :exec
INSERT INTO demo_product (product_id, session_token, slot, expires_at) VALUES (?, ?, ?, ?);

-- name: IsDemoProductOwnedBySession :one
SELECT EXISTS(
    SELECT 1 FROM demo_product dp JOIN product p ON p.id = dp.product_id
    WHERE p.public_id = ? AND dp.session_token = ? AND p.deleted_at IS NULL
);

-- name: IsDemoVariantOwnedBySession :one
SELECT EXISTS(
    SELECT 1 FROM demo_product dp
    JOIN product p ON p.id = dp.product_id
    JOIN variant v ON v.product_id = p.id
    WHERE v.public_id = ? AND dp.session_token = ? AND p.deleted_at IS NULL AND v.deleted_at IS NULL
);

-- name: IsDemoMediaOwnedBySession :one
SELECT EXISTS(
    SELECT 1 FROM demo_product dp
    JOIN media_asset m ON m.product_id = dp.product_id
    WHERE m.public_id = ? AND dp.session_token = ? AND m.deleted_at IS NULL
);

-- name: ListExpiredDemoProducts :many
SELECT p.id, p.public_id FROM demo_product dp JOIN product p ON p.id = dp.product_id
WHERE dp.expires_at <= ? ORDER BY p.id;

-- name: ListDemoProductsBySession :many
SELECT p.id, p.public_id FROM demo_product dp JOIN product p ON p.id = dp.product_id
WHERE dp.session_token = ? ORDER BY p.id;

-- name: DeleteDemoProductClaim :exec
DELETE FROM demo_product WHERE product_id = ?;

-- name: HardDeleteProduct :exec
DELETE FROM product WHERE id = ?;
