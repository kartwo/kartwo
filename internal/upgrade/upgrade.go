// 升级保护 / Upgrade Guard
// 功能：对已有数据目录在迁移前创建完整快照，并协调原子迁移批次
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 14:00:00
package upgrade

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"

	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/migrate"
)

// Result 是一次启动期升级的可观测结果。
type Result struct {
	Pending      int
	Applied      int
	SnapshotPath string
}

// Apply 仅在既有 shop.db 有待执行迁移时先创建完整 ZIP 快照，再原子应用整批迁移。
// 迁移失败时数据库事务整体回滚，快照会保留供人工恢复。
func Apply(ctx context.Context, db *sql.DB, dataDir, appVersion string, existingDatabase bool, fsys fs.FS) (Result, error) {
	pending, err := migrate.Pending(ctx, db, fsys)
	if err != nil {
		return Result{}, err
	}
	result := Result{Pending: len(pending)}
	if len(pending) == 0 {
		return result, nil
	}

	if existingDatabase {
		snapshot, _, err := backup.New(db, dataDir, appVersion).CreateUpgradeSnapshot(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("upgrade: 创建迁移前快照失败: %w", err)
		}
		result.SnapshotPath = snapshot
	}

	applied, err := migrate.Run(ctx, db, fsys)
	if err != nil {
		return result, fmt.Errorf("upgrade: 应用迁移失败，数据库已回滚: %w", err)
	}
	result.Applied = applied
	return result, nil
}

// ExistingDatabase 必须在打开 SQLite 前调用，用于区分首次安装和既有店铺升级。
func ExistingDatabase(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("upgrade: 检查数据库文件失败: %w", err)
	}
	return info.Mode().IsRegular() && info.Size() > 0, nil
}
