// Shopify 重定向服务 / Shopify Redirect Service
// 功能：在导入事务中保存旧 Handle 映射，并只为可公开商品解析店面永久重定向
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-21 14:30:00
package redirect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound 表示旧 Shopify Handle 没有可公开的重定向目标。
var ErrNotFound = errors.New("redirect: 资源不存在")

// Service 管理 Shopify 历史商品链接的映射。
type Service struct{ db *sql.DB }

// New 创建重定向服务。
func New(db *sql.DB) *Service { return &Service{db: db} }

// StoreShopifyTx 在调用方事务中保存 Handle 到商品内部 ID 的映射。
// Handle 冲突代表已有历史链接，绝不静默改写其目标。
func (s *Service) StoreShopifyTx(ctx context.Context, tx *sql.Tx, handle string, productID int64) error {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return fmt.Errorf("redirect: Shopify Handle 不能为空")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO shopify_redirect (legacy_handle, product_id) VALUES (?, ?)`, handle, productID); err != nil {
		return fmt.Errorf("redirect: 保存 Shopify 旧链接失败: %w", err)
	}
	return nil
}

// ResolveShopifyHandle 返回已上架且未删除商品当前 slug。草稿、归档和不存在均视为不可公开。
func (s *Service) ResolveShopifyHandle(ctx context.Context, handle string) (string, error) {
	var slug string
	err := s.db.QueryRowContext(ctx, `SELECT p.slug FROM shopify_redirect r JOIN product p ON p.id = r.product_id WHERE r.legacy_handle = ? AND p.status = 'active' AND p.deleted_at IS NULL`, handle).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("redirect: 查询 Shopify 旧链接失败: %w", err)
	}
	return slug, nil
}
