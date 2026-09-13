// 跑步演示封面导入 / Running Demo Cover Import
// 功能：按稳定商品 slug 幂等导入外置演示封面，保留商家已有图片
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 21:57:14
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kartwo/kartwo/internal/media"
)

// RunningDemoImageResult 记录本次演示封面导入结果。
type RunningDemoImageResult struct {
	DirectoryFound bool
	Imported       int
	Existing       int
	Missing        int
}

// ImportRunningDemoImages 优先读取 <slug>.webp、兼容旧 <slug>.png，并通过正式媒体管线生成派生图。
// 已有任意商品图片时跳过，避免覆盖商家的封面；目录不存在时安全跳过。
func (s *Service) ImportRunningDemoImages(ctx context.Context, dir string, mediaSvc *media.Service) (RunningDemoImageResult, error) {
	result := RunningDemoImageResult{}
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("catalog: 检查演示封面目录失败: %w", err)
	}
	if !info.IsDir() {
		return result, fmt.Errorf("catalog: 演示封面路径不是目录: %s", dir)
	}
	result.DirectoryFound = true

	for _, group := range runningDemoCatalog() {
		for _, product := range group.Products {
			row, err := s.q.GetProductBySlug(ctx, product.Slug)
			if errors.Is(err, sql.ErrNoRows) {
				result.Missing++
				continue
			}
			if err != nil {
				return result, fmt.Errorf("catalog: 取演示商品 %s 失败: %w", product.Slug, err)
			}
			existing, err := mediaSvc.ListByProduct(ctx, row.ID)
			if err != nil {
				return result, fmt.Errorf("catalog: 检查演示商品 %s 图片失败: %w", product.Slug, err)
			}
			if len(existing) > 0 {
				result.Existing++
				continue
			}

			imagePath := filepath.Join(dir, product.Slug+".webp")
			data, err := os.ReadFile(imagePath) //nolint:gosec // 文件名来自内置固定 slug，不接受用户输入。
			if errors.Is(err, os.ErrNotExist) {
				imagePath = filepath.Join(dir, product.Slug+".png")
				data, err = os.ReadFile(imagePath) //nolint:gosec // 兼容旧图片包；文件名仍来自内置固定 slug。
				if errors.Is(err, os.ErrNotExist) {
					result.Missing++
					continue
				}
			}
			if err != nil {
				return result, fmt.Errorf("catalog: 读取演示封面 %s 失败: %w", product.Slug, err)
			}
			asset, err := mediaSvc.Upload(ctx, row.ID, data)
			if err != nil {
				return result, fmt.Errorf("catalog: 导入演示封面 %s 失败: %w", product.Slug, err)
			}
			if err := mediaSvc.UpdateAlt(ctx, asset.PublicID, product.Title); err != nil {
				return result, fmt.Errorf("catalog: 保存演示封面 %s alt 失败: %w", product.Slug, err)
			}
			result.Imported++
		}
	}
	return result, nil
}
