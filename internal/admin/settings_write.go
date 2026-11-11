// 设置事务写入 / Transactional Settings Writes
// 功能：凭证先加密再原子保存多字段，空密钥保留旧值，失败不部分落库
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 17:48:02
package admin

import (
	"context"

	"github.com/kartwo/kartwo/internal/auth"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

type settingChange struct {
	key, value string
	encrypted  bool
}

// Callers validate every submitted block before calling this method.
// Encryption happens before the transaction; no external calls run while holding it.
func (h *HTTP) saveSettings(ctx context.Context, changes []settingChange, kek []byte) error {
	updates := make([]sqlcgen.UpsertSettingParams, 0, len(changes))
	for _, change := range changes {
		update := sqlcgen.UpsertSettingParams{Key: change.key, Value: change.value}
		if change.encrypted {
			if change.value == "" {
				continue
			}
			value, err := auth.Encrypt(kek, []byte(change.value))
			if err != nil {
				return err
			}
			update.Value, update.Encrypted = value, 1
		}
		updates = append(updates, update)
	}
	if len(updates) == 0 {
		return nil
	}
	tx, err := h.svc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := sqlcgen.New(tx)
	for _, update := range updates {
		if err := q.UpsertSetting(ctx, update); err != nil {
			return err
		}
	}
	return tx.Commit()
}
