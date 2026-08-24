// 审计日志服务测试 / Audit Log Service Tests
// 功能：验证事件只追加、按最近优先读取且不接受无效动作
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-24 00:20:00
package audit

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func TestRecordAndList(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/audit.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	q := sqlcgen.New(db)
	adminID, err := q.CreateAdminUser(context.Background(), sqlcgen.CreateAdminUserParams{PublicID: "admin-public-id", Username: "admin", PasswordHash: "unused"})
	if err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	svc := New(db)
	if err := svc.Record(context.Background(), adminID, "product.create", "product", "product-1"); err != nil {
		t.Fatalf("记录事件失败: %v", err)
	}
	if err := svc.Record(context.Background(), adminID, "product.delete", "product", "product-2"); err != nil {
		t.Fatalf("记录事件失败: %v", err)
	}
	events, err := svc.List(context.Background(), 100)
	if err != nil {
		t.Fatalf("读取事件失败: %v", err)
	}
	if len(events) != 2 || events[0].Action != "product.delete" || events[0].TargetPublicID != "product-2" {
		t.Fatalf("事件应按最新优先完整返回，得 %+v", events)
	}
	if events[0].AdminUsername != "admin" || events[0].AdminPublicID != "admin-public-id" {
		t.Fatalf("事件应包含操作人，得 %+v", events[0])
	}
	if err := svc.Record(context.Background(), adminID, "", "product", "product-3"); err == nil {
		t.Fatal("空动作必须拒绝")
	}
}
