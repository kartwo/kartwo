// 无人值守业务密钥 / Unattended Business Key
// 功能：以数据目录外的受限本机钥匙封装店主 KEK，重启后自动解锁收款与邮件缓存
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-09 11:32:00
package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kartwo/kartwo/internal/auth"
)

const runtimeWrappedKEK = "runtime.wrapped_kek"

// SetRuntimeKeyPath 只在服务装配时调用；路径必须位于数据目录外。
func (s *Service) SetRuntimeKeyPath(path string) { s.runtimeKeyPath = filepath.Clean(path) }

func readOrCreateRuntimeKey(path string) ([]byte, error) {
	if path == "" || path == "." {
		return nil, fmt.Errorf("admin: 无人值守钥匙路径为空")
	}
	key := make([]byte, 32)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- 路径由服务启动时的数据目录配置决定
	if err == nil {
		if _, err = rand.Read(key); err == nil {
			_, err = file.Write(key)
		}
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return nil, fmt.Errorf("admin: 创建无人值守钥匙失败: %w", err)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("admin: 打开无人值守钥匙失败: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("admin: 无人值守钥匙须是仅当前用户可读写的普通文件")
	}
	key, err = os.ReadFile(path) // #nosec G304 -- 已验证普通文件及私有权限
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("admin: 无人值守钥匙长度无效")
	}
	return key, nil
}

// pairRuntimeKey 用已验证的店主口令派生出的 KEK 建立或修复本机封装。
// 迁店后新机器缺少旧钥匙时，首次店主登录可重新配对。
func (s *Service) pairRuntimeKey(ctx context.Context, kek []byte) error {
	if s.runtimeKeyPath == "" {
		return nil
	}
	machineKey, err := readOrCreateRuntimeKey(s.runtimeKeyPath)
	if err != nil {
		return err
	}
	defer clearBytes(machineKey)
	var wrapped string
	err = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, runtimeWrappedKEK).Scan(&wrapped)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		existing, decryptErr := auth.Decrypt(machineKey, wrapped)
		if decryptErr == nil {
			same := subtle.ConstantTimeCompare(existing, kek) == 1
			clearBytes(existing)
			if same {
				s.unattended.Store(true)
				return nil
			}
		}
	}
	sealed, err := auth.Encrypt(machineKey, kek)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, runtimeWrappedKEK, sealed); err != nil {
		return err
	}
	s.unattended.Store(true)
	return nil
}

// AutoUnlockBusinessKeys 在服务启动时尝试恢复业务缓存；未配对的旧店仍可登录后配对。
func (s *Service) AutoUnlockBusinessKeys(ctx context.Context) (bool, error) {
	if s.runtimeKeyPath == "" {
		return false, nil
	}
	var wrapped string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, runtimeWrappedKEK).Scan(&wrapped)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	machineKey, err := readOrCreateRuntimeKey(s.runtimeKeyPath)
	if err != nil {
		return false, err
	}
	defer clearBytes(machineKey)
	kek, err := auth.Decrypt(machineKey, wrapped)
	if err != nil {
		return false, fmt.Errorf("admin: 无人值守钥匙与数据不匹配，请店主登录重新配对: %w", err)
	}
	defer clearBytes(kek)
	if s.keys != nil {
		if err := s.keys.Unlock(ctx, kek); err != nil {
			return false, err
		}
	}
	if s.mailKeys != nil {
		if err := s.mailKeys.Unlock(ctx, kek); err != nil {
			return false, err
		}
	}
	s.unattended.Store(true)
	return true, nil
}

func (s *Service) rotateRuntimeKeyTx(ctx context.Context, tx *sql.Tx, kek []byte) error {
	if s.runtimeKeyPath == "" || !s.unattended.Load() {
		return nil
	}
	machineKey, err := readOrCreateRuntimeKey(s.runtimeKeyPath)
	if err != nil {
		return err
	}
	defer clearBytes(machineKey)
	sealed, err := auth.Encrypt(machineKey, kek)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, runtimeWrappedKEK, sealed)
	return err
}

func clearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
