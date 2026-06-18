// 媒体存储后端 / Media Storage Backend
// 功能：可插拔存储后端接口 + 本地默认实现（S3/R2 为 v1.1，仅留接口）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
package media

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Backend 是可插拔媒体存储后端（双模式分叉点；默认 Local，S3/R2 后置）。
type Backend interface {
	Put(relPath string, data []byte) error
	Open(relPath string) (io.ReadCloser, error)
	Remove(relPath string) error
	Root() string
}

// LocalBackend 把媒体落到本地目录（默认 ./data/media）。
type LocalBackend struct {
	root string
}

// NewLocalBackend 构造本地后端，root 为媒体根目录。
func NewLocalBackend(root string) *LocalBackend {
	return &LocalBackend{root: root}
}

func (b *LocalBackend) Root() string { return b.root }

// safeJoin 防路径穿越：相对路径不得逃出 root。
func (b *LocalBackend) safeJoin(relPath string) (string, error) {
	clean := filepath.Clean("/" + relPath) // 归一化，去掉 ../
	full := filepath.Join(b.root, clean)
	if full != b.root && !strings.HasPrefix(full, b.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("media: 非法路径 %q", relPath)
	}
	return full, nil
}

func (b *LocalBackend) Put(relPath string, data []byte) error {
	full, err := b.safeJoin(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("media: 建目录失败: %w", err)
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return fmt.Errorf("media: 写文件失败: %w", err)
	}
	return nil
}

func (b *LocalBackend) Open(relPath string) (io.ReadCloser, error) {
	full, err := b.safeJoin(relPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full) //nolint:gosec // safeJoin 已防穿越
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (b *LocalBackend) Remove(relPath string) error {
	full, err := b.safeJoin(relPath)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("media: 删文件失败: %w", err)
	}
	return nil
}
