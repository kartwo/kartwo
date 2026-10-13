// 博客图片存储 / Blog Image Storage
// 功能：复用图片校验、去元数据和 WebP 管线保存博客封面及正文图片
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package media

import (
	"fmt"
	"path"
)

type BlogImage struct {
	Path, URL     string
	Width, Height int
}

func (s *Service) StoreBlogImage(data []byte) (*BlogImage, error) {
	if err := s.policy.AllowUpload(int64(len(data))); err != nil {
		return nil, err
	}
	p, err := Process(data)
	if err != nil {
		return nil, err
	}
	var selected *Derivative
	for i := range p.Derivatives {
		if p.Derivatives[i].Label == "large" {
			selected = &p.Derivatives[i]
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("media: 博客图片生成失败")
	}
	rel := path.Join("blog", p.ContentHash+".webp")
	if err := s.backend.Put(rel, selected.Bytes); err != nil {
		return nil, err
	}
	return &BlogImage{Path: rel, URL: mediaURL(rel), Width: selected.Width, Height: selected.Height}, nil
}
