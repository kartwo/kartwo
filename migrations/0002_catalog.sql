-- 商品目录核心模型 / Catalog Core Schema
-- 功能：商品/通用变体轴(option×value)/变体/库存/分类；双轴枚举变体，不写死品类
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-17 18:13:43
-- 说明：纯 SQL 迁移、幂等可重入；金额整数(分)；外部 ID 用 UUIDv7(public_id)，内部主键自增不对外；
--       统一 created_at/updated_at/deleted_at(软删)，时间为 UTC ISO8601 文本。

-- 商品
CREATE TABLE IF NOT EXISTS product (
    id          INTEGER PRIMARY KEY,
    public_id   TEXT NOT NULL UNIQUE,                 -- UUIDv7，对外暴露
    title       TEXT NOT NULL,
    slug        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',         -- draft | active | archived
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    deleted_at  TEXT
);
-- slug 在未软删范围内唯一
CREATE UNIQUE INDEX IF NOT EXISTS ux_product_slug ON product(slug) WHERE deleted_at IS NULL;

-- 变体轴（如"尺码"、"颜色"），一个商品可有多个轴
CREATE TABLE IF NOT EXISTS product_option (
    id         INTEGER PRIMARY KEY,
    product_id INTEGER NOT NULL REFERENCES product(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (product_id, name)
);

-- 轴上的取值（如 S/M/L、黑/白）
CREATE TABLE IF NOT EXISTS product_option_value (
    id         INTEGER PRIMARY KEY,
    option_id  INTEGER NOT NULL REFERENCES product_option(id) ON DELETE CASCADE,
    value      TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (option_id, value)
);

-- 变体：可售单元 = 各轴各取一个值的组合，自带 SKU/价格/库存
CREATE TABLE IF NOT EXISTS variant (
    id          INTEGER PRIMARY KEY,
    public_id   TEXT NOT NULL UNIQUE,                  -- UUIDv7
    product_id  INTEGER NOT NULL REFERENCES product(id) ON DELETE CASCADE,
    sku         TEXT,                                  -- 可空；非空时唯一
    price_cents INTEGER NOT NULL DEFAULT 0,            -- 金额整数(分)
    -- option_key：该变体所含选项值的规范化签名（按 option_id 升序拼 value_id），
    -- 用于在商品内强制"选项值组合唯一"，避免重复变体。
    option_key  TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    deleted_at  TEXT
);
-- 同一商品下变体的选项组合唯一（未软删范围内）
CREATE UNIQUE INDEX IF NOT EXISTS ux_variant_combo ON variant(product_id, option_key) WHERE deleted_at IS NULL;
-- SKU 非空时全局唯一（未软删范围内）
CREATE UNIQUE INDEX IF NOT EXISTS ux_variant_sku ON variant(sku) WHERE sku IS NOT NULL AND deleted_at IS NULL;

-- 变体↔选项值连接：每个变体在每个轴上恰好一个取值
CREATE TABLE IF NOT EXISTS variant_option_value (
    variant_id INTEGER NOT NULL REFERENCES variant(id) ON DELETE CASCADE,
    option_id  INTEGER NOT NULL REFERENCES product_option(id) ON DELETE CASCADE,
    value_id   INTEGER NOT NULL REFERENCES product_option_value(id) ON DELETE CASCADE,
    PRIMARY KEY (variant_id, option_id)
);

-- 库存：按变体记数量；reserved 为 M2 防超卖预留打底
CREATE TABLE IF NOT EXISTS inventory (
    variant_id INTEGER PRIMARY KEY REFERENCES variant(id) ON DELETE CASCADE,
    quantity   INTEGER NOT NULL DEFAULT 0,
    reserved   INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    CHECK (quantity >= 0),
    CHECK (reserved >= 0)
);

-- 分类（支持父子层级）
CREATE TABLE IF NOT EXISTS category (
    id         INTEGER PRIMARY KEY,
    public_id  TEXT NOT NULL UNIQUE,                   -- UUIDv7
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    parent_id  INTEGER REFERENCES category(id) ON DELETE SET NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_category_slug ON category(slug) WHERE deleted_at IS NULL;

-- 商品↔分类（多对多）
CREATE TABLE IF NOT EXISTS product_category (
    product_id  INTEGER NOT NULL REFERENCES product(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES category(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);
