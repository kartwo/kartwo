// 店面服务 / Storefront Service
// 功能：组装店面只读视图（仅 active 商品）——目录列表、商品详情(变体/图/价/库存)
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 11:20:00
package storefront

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// ErrNotFound 商品不存在或未上架。
var ErrNotFound = errors.New("storefront: 商品不存在")

// Service 承载店面只读数据组装。
type Service struct {
	q *sqlcgen.Queries
}

// New 构造店面服务。
func New(db *sql.DB) *Service { return &Service{q: sqlcgen.New(db)} }

// CatalogItem 是目录列表中的一项。
type CatalogItem struct {
	Title       string
	Slug        string
	Description string
	FromCents   int64
	ThumbURL    string
	ThumbAlt    string
	UpdatedAt   string
	Category    string
}

// Category 是店面可浏览分类及其上架商品数量。
type Category struct {
	Name         string
	Slug         string
	ProductCount int64
}

// Image 是一张图的多尺寸 URL 集合。
type Image struct {
	Thumb  string
	Medium string
	Large  string
	Alt    string
	W, H   int
}

// Variant 是详情页的可售单元。
type Variant struct {
	PublicID  string
	SKU       string
	Cents     int64
	Available int64 // quantity - reserved
	Options   []OptionPair
}

type OptionPair struct{ Name, Value string }

// ProductPage 是商品详情页所需数据。
type ProductPage struct {
	Title          string
	Slug           string
	Description    string
	SEODescription string
	PublicID       string
	UpdatedAt      string
	MinCents       int64
	MaxCents       int64
	InStock        bool
	Images         []Image
	Variants       []Variant
	Categories     []Category
}

// ListCatalog 返回所有上架商品（含起价与首图缩略）。
func (s *Service) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	rows, err := s.q.ListActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列目录失败: %w", err)
	}
	out := make([]CatalogItem, 0, len(rows))
	for _, r := range rows {
		item, err := s.catalogItem(ctx, r.ID, r.Title, r.Slug, r.Description, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// ListFeaturedCatalog 返回商家手动选中的上架商品；精选为空时调用方可诚实回退到普通目录。
func (s *Service) ListFeaturedCatalog(ctx context.Context) ([]CatalogItem, error) {
	rows, err := s.q.ListFeaturedActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列精选目录失败: %w", err)
	}
	out := make([]CatalogItem, 0, len(rows))
	for _, r := range rows {
		item, err := s.catalogItem(ctx, r.ID, r.Title, r.Slug, r.Description, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type ContentPage struct{ Title, Slug, BodyMarkdown, SEODescription, UpdatedAt string }

// ListContentPages 返回已发布内容页，供正式店面页脚与 sitemap 使用。
func (s *Service) ListContentPages(ctx context.Context) ([]ContentPage, error) {
	rows, err := s.q.ListActiveContentPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列内容页失败: %w", err)
	}
	out := make([]ContentPage, 0, len(rows))
	for _, p := range rows {
		out = append(out, ContentPage{p.Title, p.Slug, p.BodyMarkdown, p.SeoDescription, p.UpdatedAt})
	}
	return out, nil
}

// ListCategories 返回至少含一件上架商品的分类，避免店面出现空分类。
func (s *Service) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.q.ListStorefrontCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列分类失败: %w", err)
	}
	out := make([]Category, 0, len(rows))
	for _, row := range rows {
		out = append(out, Category{Name: row.Name, Slug: row.Slug, ProductCount: row.ProductCount})
	}
	return out, nil
}

// ListCatalogByCategory 返回指定分类及其上架商品。
func (s *Service) ListCatalogByCategory(ctx context.Context, slug string) (*Category, []CatalogItem, error) {
	cat, err := s.q.GetCategoryBySlug(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("storefront: 取分类失败: %w", err)
	}
	rows, err := s.q.ListActiveProductsByCategory(ctx, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("storefront: 列分类商品失败: %w", err)
	}
	items := make([]CatalogItem, 0, len(rows))
	for _, row := range rows {
		item, err := s.catalogItem(ctx, row.ID, row.Title, row.Slug, row.Description, row.UpdatedAt)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	return &Category{Name: cat.Name, Slug: cat.Slug, ProductCount: int64(len(items))}, items, nil
}

// SearchCatalog 搜索上架商品标题与描述。
func (s *Service) SearchCatalog(ctx context.Context, term string) ([]CatalogItem, error) {
	rows, err := s.q.SearchActiveProducts(ctx, term)
	if err != nil {
		return nil, fmt.Errorf("storefront: 搜索商品失败: %w", err)
	}
	items := make([]CatalogItem, 0, len(rows))
	for _, row := range rows {
		item, err := s.catalogItem(ctx, row.ID, row.Title, row.Slug, row.Description, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) GetContentPage(ctx context.Context, slug string) (*ContentPage, error) {
	p, err := s.q.GetActiveContentPageBySlug(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storefront: 取内容页失败: %w", err)
	}
	return &ContentPage{p.Title, p.Slug, p.BodyMarkdown, p.SeoDescription, p.UpdatedAt}, nil
}

// GetProduct 组装商品详情。未上架/不存在返回 ErrNotFound。
func (s *Service) GetProduct(ctx context.Context, slug string) (*ProductPage, error) {
	p, err := s.q.GetActiveProductBySlug(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("storefront: 取商品失败: %w", err)
	}
	page := &ProductPage{
		Title: p.Title, Slug: p.Slug, Description: p.Description, SEODescription: p.SeoDescription, PublicID: p.PublicID, UpdatedAt: p.UpdatedAt,
	}

	// 变体 + 选项 + 库存。
	vrows, err := s.q.ListVariantsByProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列变体失败: %w", err)
	}
	pairs, err := s.q.ListVariantOptionValuesByProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列变体选项失败: %w", err)
	}
	optByVariant := map[int64][]OptionPair{}
	for _, pr := range pairs {
		optByVariant[pr.VariantID] = append(optByVariant[pr.VariantID], OptionPair{Name: pr.OptionName, Value: pr.OptionValue})
	}
	page.MinCents, page.MaxCents = -1, -1
	for _, v := range vrows {
		inv, err := s.q.GetInventory(ctx, v.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("storefront: 取库存失败: %w", err)
		}
		avail := inv.Quantity - inv.Reserved
		if avail < 0 {
			avail = 0
		}
		if avail > 0 {
			page.InStock = true
		}
		sku := ""
		if v.Sku.Valid {
			sku = v.Sku.String
		}
		page.Variants = append(page.Variants, Variant{
			PublicID: v.PublicID, SKU: sku, Cents: v.PriceCents, Available: avail, Options: optByVariant[v.ID],
		})
		if page.MinCents < 0 || v.PriceCents < page.MinCents {
			page.MinCents = v.PriceCents
		}
		if v.PriceCents > page.MaxCents {
			page.MaxCents = v.PriceCents
		}
	}
	if page.MinCents < 0 {
		page.MinCents, page.MaxCents = 0, 0
	}

	// 图片（多尺寸）。
	imgs, err := s.images(ctx, p.ID, p.Title)
	if err != nil {
		return nil, err
	}
	page.Images = imgs
	categoryIDs, err := s.q.ListCategoryPublicIDsByProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列商品分类失败: %w", err)
	}
	for _, id := range categoryIDs {
		cat, err := s.q.GetCategoryByPublicID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("storefront: 取商品分类失败: %w", err)
		}
		page.Categories = append(page.Categories, Category{Name: cat.Name, Slug: cat.Slug})
	}
	return page, nil
}

func (s *Service) catalogItem(ctx context.Context, productID int64, title, slug, description, updatedAt string) (CatalogItem, error) {
	item := CatalogItem{Title: title, Slug: slug, Description: description, UpdatedAt: updatedAt}
	if cents, ok, err := s.minPrice(ctx, productID); err != nil {
		return CatalogItem{}, err
	} else if ok {
		item.FromCents = cents
	}
	if thumb, err := s.firstThumb(ctx, productID, title); err != nil {
		return CatalogItem{}, err
	} else {
		item.ThumbURL, item.ThumbAlt = thumb.Medium, thumb.Alt
	}
	ids, err := s.q.ListCategoryPublicIDsByProduct(ctx, productID)
	if err != nil {
		return CatalogItem{}, fmt.Errorf("storefront: 列商品分类失败: %w", err)
	}
	if len(ids) > 0 {
		cat, err := s.q.GetCategoryByPublicID(ctx, ids[0])
		if err != nil {
			return CatalogItem{}, fmt.Errorf("storefront: 取商品分类失败: %w", err)
		}
		item.Category = cat.Name
	}
	return item, nil
}

func (s *Service) minPrice(ctx context.Context, productID int64) (int64, bool, error) {
	vrows, err := s.q.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return 0, false, fmt.Errorf("storefront: 列变体失败: %w", err)
	}
	min := int64(-1)
	for _, v := range vrows {
		if min < 0 || v.PriceCents < min {
			min = v.PriceCents
		}
	}
	if min < 0 {
		return 0, false, nil
	}
	return min, true, nil
}

func (s *Service) firstThumb(ctx context.Context, productID int64, fallbackAlt string) (Image, error) {
	imgs, err := s.images(ctx, productID, fallbackAlt)
	if err != nil {
		return Image{}, err
	}
	if len(imgs) == 0 {
		return Image{}, nil
	}
	return imgs[0], nil
}

func (s *Service) images(ctx context.Context, productID int64, fallbackAlt string) ([]Image, error) {
	assets, err := s.q.ListMediaByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列图片失败: %w", err)
	}
	out := make([]Image, 0, len(assets))
	for _, a := range assets {
		ds, err := s.q.ListDerivativesByAsset(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("storefront: 列派生失败: %w", err)
		}
		img := Image{Alt: firstNonEmpty(a.AltText, fallbackAlt)}
		for _, d := range ds {
			url := "/media/" + d.Path
			switch d.Label {
			case "thumb":
				img.Thumb = url
			case "medium":
				img.Medium = url
			case "large":
				img.Large, img.W, img.H = url, int(d.Width), int(d.Height)
			}
		}
		// 兜底：缺某尺寸时用其它填充，保证有可显示的 URL。
		if img.Medium == "" {
			img.Medium = firstNonEmpty(img.Large, img.Thumb)
		}
		if img.Large == "" {
			img.Large = firstNonEmpty(img.Medium, img.Thumb)
		}
		if img.Thumb == "" {
			img.Thumb = firstNonEmpty(img.Medium, img.Large)
		}
		out = append(out, img)
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
