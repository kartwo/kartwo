// 内容页服务 / Content Page Service
// 功能：管理公开品牌内容页；正文只保存受限 Markdown 源文本
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-07 12:00:00
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

type ContentPage struct{ PublicID, Title, Slug, BodyMarkdown, SEODescription, Status, UpdatedAt string }

func validateContent(title, slug, status string) error {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(slug) == "" {
		return vErr("页面标题与 slug 不能为空")
	}
	if status != "draft" && status != "active" {
		return vErr("页面状态非法（应为 draft/active）")
	}
	return nil
}

func (s *Service) CreateContentPage(ctx context.Context, title, slug, body, seo, status string) (string, error) {
	if err := validateContent(title, slug, status); err != nil {
		return "", err
	}
	id := uuid.Must(uuid.NewV7()).String()
	_, err := s.q.CreateContentPage(ctx, sqlcgen.CreateContentPageParams{PublicID: id, Title: title, Slug: slug, BodyMarkdown: body, SeoDescription: seo, Status: status})
	if isUnique(err) {
		return "", vErr("页面 slug %q 已存在", slug)
	}
	if err != nil {
		return "", fmt.Errorf("catalog: 建内容页失败: %w", err)
	}
	return id, nil
}

func (s *Service) ListContentPages(ctx context.Context) ([]ContentPage, error) {
	rows, err := s.q.ListContentPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: 列内容页失败: %w", err)
	}
	out := make([]ContentPage, 0, len(rows))
	for _, r := range rows {
		out = append(out, ContentPage{r.PublicID, r.Title, r.Slug, r.BodyMarkdown, r.SeoDescription, r.Status, r.UpdatedAt})
	}
	return out, nil
}
func (s *Service) GetContentPage(ctx context.Context, id string) (*ContentPage, error) {
	r, err := s.q.GetContentPageByPublicID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: 取内容页失败: %w", err)
	}
	return &ContentPage{r.PublicID, r.Title, r.Slug, r.BodyMarkdown, r.SeoDescription, r.Status, r.UpdatedAt}, nil
}
func (s *Service) UpdateContentPage(ctx context.Context, id, title, body, seo, status string) error {
	p, err := s.q.GetContentPageByPublicID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("catalog: 取内容页失败: %w", err)
	}
	if err := validateContent(title, p.Slug, status); err != nil {
		return err
	}
	return s.q.UpdateContentPage(ctx, sqlcgen.UpdateContentPageParams{Title: title, BodyMarkdown: body, SeoDescription: seo, Status: status, ID: p.ID})
}

func (s *Service) DeleteContentPage(ctx context.Context, id string) error {
	p, err := s.q.GetContentPageByPublicID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("catalog: 取内容页失败: %w", err)
	}
	return s.q.SoftDeleteContentPage(ctx, p.ID)
}
