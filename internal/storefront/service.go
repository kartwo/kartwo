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
	Title         string
	Slug          string
	Description   string
	FromCents     int64
	ThumbURL      string
	UpdatedAt     string
}

// Image 是一张图的多尺寸 URL 集合。
type Image struct {
	Thumb  string
	Medium string
	Large  string
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
	Title       string
	Slug        string
	Description string
	PublicID    string
	UpdatedAt   string
	MinCents    int64
	MaxCents    int64
	InStock     bool
	Images      []Image
	Variants    []Variant
}

// ListCatalog 返回所有上架商品（含起价与首图缩略）。
func (s *Service) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	rows, err := s.q.ListActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("storefront: 列目录失败: %w", err)
	}
	out := make([]CatalogItem, 0, len(rows))
	for _, r := range rows {
		item := CatalogItem{Title: r.Title, Slug: r.Slug, Description: r.Description, UpdatedAt: r.UpdatedAt}
		if cents, ok, err := s.minPrice(ctx, r.ID); err != nil {
			return nil, err
		} else if ok {
			item.FromCents = cents
		}
		if thumb, err := s.firstThumb(ctx, r.ID); err != nil {
			return nil, err
		} else {
			item.ThumbURL = thumb
		}
		out = append(out, item)
	}
	return out, nil
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
		Title: p.Title, Slug: p.Slug, Description: p.Description, PublicID: p.PublicID, UpdatedAt: p.UpdatedAt,
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
	imgs, err := s.images(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	page.Images = imgs
	return page, nil
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

func (s *Service) firstThumb(ctx context.Context, productID int64) (string, error) {
	imgs, err := s.images(ctx, productID)
	if err != nil {
		return "", err
	}
	if len(imgs) == 0 {
		return "", nil
	}
	if imgs[0].Thumb != "" {
		return imgs[0].Thumb, nil
	}
	return imgs[0].Medium, nil
}

func (s *Service) images(ctx context.Context, productID int64) ([]Image, error) {
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
		img := Image{}
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
