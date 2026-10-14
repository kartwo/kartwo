// 博客服务 / Blog Service
// 功能：文章草稿、发布、列表与默认开启的店面开关
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/settings"
)

var ErrNotFound = errors.New("blog: 文章不存在")
var ErrInvalid = errors.New("blog: 文章内容不合法")
var ErrConflict = errors.New("blog: 链接已存在")

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var imagePathPattern = regexp.MustCompile(`^blog/[a-f0-9]{64}\.webp$`)

type Post struct {
	PublicID       string `json:"public_id"`
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Excerpt        string `json:"excerpt"`
	BodyMarkdown   string `json:"body_markdown"`
	BodyHTML       string `json:"body_html"`
	CoverPath      string `json:"cover_path"`
	SEODescription string `json:"seo_description"`
	Status         string `json:"status"`
	PublishedAt    string `json:"published_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (p Post) CoverURL() string {
	if imagePathPattern.MatchString(p.CoverPath) {
		return "/media/" + p.CoverPath
	}
	return ""
}
func (p Post) PublishedDate() string {
	if len(p.PublishedAt) >= 10 {
		return p.PublishedAt[:10]
	}
	return p.PublishedAt
}

type Service struct {
	db       *sql.DB
	settings *settings.Service
}

func New(db *sql.DB, settingsSvc *settings.Service) *Service {
	return &Service{db: db, settings: settingsSvc}
}

func (s *Service) Enabled(ctx context.Context) (bool, error) {
	v, err := s.settings.Get(ctx, "blog.enabled")
	if errors.Is(err, settings.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return v == "1", nil
}
func (s *Service) SetEnabled(ctx context.Context, enabled bool) error {
	v := "0"
	if enabled {
		v = "1"
	}
	return s.settings.SetPlain(ctx, "blog.enabled", v)
}

func validate(p *Post) error {
	if n := len([]rune(strings.TrimSpace(p.Title))); n == 0 || n > 160 {
		return fmt.Errorf("%w: 标题需为 1–160 字", ErrInvalid)
	}
	if len(p.Slug) > 100 || !slugPattern.MatchString(p.Slug) {
		return fmt.Errorf("%w: 链接只能用小写英文、数字和连字符", ErrInvalid)
	}
	if len([]rune(p.Excerpt)) > 300 || len([]rune(p.SEODescription)) > 300 || len(p.BodyMarkdown) > 100000 || len(p.BodyHTML) > 100000 {
		return fmt.Errorf("%w: 正文、摘要或 SEO 描述过长", ErrInvalid)
	}
	visibleHTML := false
	if p.BodyHTML != "" {
		p.BodyHTML, visibleHTML = SanitizeHTML(p.BodyHTML)
		if !visibleHTML {
			p.BodyHTML = ""
		}
	}
	if p.CoverPath != "" && !imagePathPattern.MatchString(p.CoverPath) {
		return fmt.Errorf("%w: 封面路径非法", ErrInvalid)
	}
	if p.Status != "draft" && p.Status != "active" {
		return fmt.Errorf("%w: 状态非法", ErrInvalid)
	}
	if p.Status == "active" && !visibleHTML && strings.TrimSpace(p.BodyMarkdown) == "" {
		return fmt.Errorf("%w: 发布前请填写正文", ErrInvalid)
	}
	return nil
}

func uniqueErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func (s *Service) Create(ctx context.Context, p Post) (string, error) {
	if err := validate(&p); err != nil {
		return "", err
	}
	id := uuid.Must(uuid.NewV7()).String()
	_, err := s.db.ExecContext(ctx, `INSERT INTO blog_post(public_id,title,slug,excerpt,body_markdown,body_html,cover_path,seo_description,status,published_at)
	VALUES(?,?,?,?,?,?,?,?,?,CASE WHEN ?='active' THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE NULL END)`, id, strings.TrimSpace(p.Title), p.Slug, p.Excerpt, p.BodyMarkdown, p.BodyHTML, p.CoverPath, p.SEODescription, p.Status, p.Status)
	if uniqueErr(err) {
		return "", ErrConflict
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id string, p Post) error {
	if err := validate(&p); err != nil {
		return err
	}
	old, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if old.Slug != p.Slug {
		return fmt.Errorf("%w: 文章链接创建后不可修改", ErrInvalid)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE blog_post SET title=?,excerpt=?,body_markdown=?,body_html=?,cover_path=?,seo_description=?,status=?,
	 published_at=CASE WHEN ?='active' AND status='draft' THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE published_at END,
	 updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND deleted_at IS NULL`, strings.TrimSpace(p.Title), p.Excerpt, p.BodyMarkdown, p.BodyHTML, p.CoverPath, p.SEODescription, p.Status, p.Status, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	r, err := s.db.ExecContext(ctx, `UPDATE blog_post SET deleted_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const postColumns = `public_id,title,slug,excerpt,body_markdown,body_html,cover_path,seo_description,status,COALESCE(published_at,''),updated_at`

func scan(row interface{ Scan(...any) error }) (Post, error) {
	var p Post
	err := row.Scan(&p.PublicID, &p.Title, &p.Slug, &p.Excerpt, &p.BodyMarkdown, &p.BodyHTML, &p.CoverPath, &p.SEODescription, &p.Status, &p.PublishedAt, &p.UpdatedAt)
	return p, err
}

func (s *Service) Get(ctx context.Context, id string) (Post, error) {
	p, err := scan(s.db.QueryRowContext(ctx, `SELECT `+postColumns+` FROM blog_post WHERE public_id=? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	return p, err
}
func (s *Service) GetPublished(ctx context.Context, slug string) (Post, error) {
	p, err := scan(s.db.QueryRowContext(ctx, `SELECT `+postColumns+` FROM blog_post WHERE slug=? AND status='active' AND deleted_at IS NULL`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	return p, err
}
func list(rows *sql.Rows) ([]Post, error) {
	defer func() { _ = rows.Close() }()
	posts := []Post{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}
func (s *Service) ListAdmin(ctx context.Context) ([]Post, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT public_id,title,slug,excerpt,'' AS body_markdown,'' AS body_html,cover_path,seo_description,status,COALESCE(published_at,''),updated_at FROM blog_post WHERE deleted_at IS NULL ORDER BY updated_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	return list(rows)
}
func (s *Service) ListPublished(ctx context.Context, page int) ([]Post, int, error) {
	if page < 1 || page > 100000 {
		return nil, 0, ErrInvalid
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blog_post WHERE status='active' AND deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+postColumns+` FROM blog_post WHERE status='active' AND deleted_at IS NULL ORDER BY published_at DESC,id DESC LIMIT 10 OFFSET ?`, (page-1)*10)
	if err != nil {
		return nil, 0, err
	}
	posts, err := list(rows)
	return posts, total, err
}
func (s *Service) HasPublished(ctx context.Context) (bool, error) {
	var found bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM blog_post WHERE status='active' AND deleted_at IS NULL)`).Scan(&found)
	return found, err
}
func (s *Service) PublishedSitemap(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slug FROM blog_post WHERE status='active' AND deleted_at IS NULL ORDER BY published_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	slugs := []string{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}
