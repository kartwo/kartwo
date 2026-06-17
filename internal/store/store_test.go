// 数据层与 sqlc 管线测试 / Store & sqlc Pipeline Test
// 功能：验证 Open→迁移→sqlc 生成代码读写 meta 全链路打通（数据层选型落地验证）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package store_test

import (
	"context"
	"testing"

	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/store"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
	"github.com/kartwo/kartwo/migrations"
)

func TestStore_SqlcPipeline(t *testing.T) {
	cfg := &config.Config{
		Env:      "dev",
		Addr:     ":0",
		DataDir:  t.TempDir(),
		DBEngine: "sqlite",
		DBPath:   t.TempDir() + "/shop.db",
	}

	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("打开数据层失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	if _, err := migrate.Run(ctx, st.DB, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	q := sqlcgen.New(st.DB)
	if err := q.UpsertMeta(ctx, sqlcgen.UpsertMetaParams{Key: "schema_label", Value: "m0"}); err != nil {
		t.Fatalf("UpsertMeta 失败: %v", err)
	}
	got, err := q.GetMeta(ctx, "schema_label")
	if err != nil {
		t.Fatalf("GetMeta 失败: %v", err)
	}
	if got.Value != "m0" {
		t.Fatalf("meta 值 = %q，期望 m0", got.Value)
	}

	// 验证 upsert 覆盖路径。
	if err := q.UpsertMeta(ctx, sqlcgen.UpsertMetaParams{Key: "schema_label", Value: "m0-updated"}); err != nil {
		t.Fatalf("二次 UpsertMeta 失败: %v", err)
	}
	got2, err := q.GetMeta(ctx, "schema_label")
	if err != nil {
		t.Fatalf("二次 GetMeta 失败: %v", err)
	}
	if got2.Value != "m0-updated" {
		t.Fatalf("更新后 meta 值 = %q，期望 m0-updated", got2.Value)
	}
}
