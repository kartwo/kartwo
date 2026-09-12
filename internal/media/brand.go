// 品牌 Logo 存储 / Brand Logo Storage
// 功能：校验并处理店铺 Logo，保存适合网页页眉的透明 WebP 文件
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 22:50:00
package media

import (
	"fmt"
	"path"
	"strings"
)

// BrandLogo 是已落盘的店铺 Logo。
type BrandLogo struct {
	Path   string
	URL    string
	Width  int
	Height int
}

// StoreBrandLogo 复用正式图片管线去元数据并输出 WebP；Logo 不写商品媒体表。
func (s *Service) StoreBrandLogo(data []byte) (*BrandLogo, error) {
	if err := s.policy.AllowUpload(int64(len(data))); err != nil {
		return nil, err
	}
	processed, err := Process(data)
	if err != nil {
		return nil, err
	}
	var selected *Derivative
	for i := range processed.Derivatives {
		if processed.Derivatives[i].Label == "medium" {
			selected = &processed.Derivatives[i]
			break
		}
		if processed.Derivatives[i].Label == "large" {
			selected = &processed.Derivatives[i]
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("media: Logo 派生图生成失败")
	}
	relPath := path.Join("brand", processed.ContentHash+".webp")
	if err := s.backend.Put(relPath, selected.Bytes); err != nil {
		return nil, err
	}
	return &BrandLogo{Path: relPath, URL: mediaURL(relPath), Width: selected.Width, Height: selected.Height}, nil
}

// DeleteBrandLogo 删除品牌目录中的旧 Logo；拒绝删除品牌目录外的任意文件。
func (s *Service) DeleteBrandLogo(relPath string) error {
	clean := path.Clean(strings.TrimSpace(relPath))
	if clean == "." || !strings.HasPrefix(clean, "brand/") {
		return fmt.Errorf("media: 非法品牌 Logo 路径")
	}
	return s.backend.Remove(clean)
}
