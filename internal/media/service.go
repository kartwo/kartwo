// 媒体服务 / Media Service
// 功能：上传(校验/处理/落盘/记库)、列表、删除、孤儿清理；StoragePolicy + 单商品张数护栏
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var (
	// ErrNotFound 媒体不存在。
	ErrNotFound = errors.New("media: 资源不存在")
	// ErrTooManyPerProduct 单商品图片数量超上限。
	ErrTooManyPerProduct = errors.New("media: 该商品图片数量已达上限")
)

// Service 承载媒体上传与生命周期。
type Service struct {
	db            *sql.DB
	q             *sqlcgen.Queries
	backend       Backend
	policy        StoragePolicy
	maxPerProduct int
}

// New 构造媒体服务。
func New(db *sql.DB, backend Backend, policy StoragePolicy, maxPerProduct int) *Service {
	return &Service{db: db, q: sqlcgen.New(db), backend: backend, policy: policy, maxPerProduct: maxPerProduct}
}

// Asset 是返回给上层的媒体视图。
type Asset struct {
	PublicID    string
	Mime        string
	Width       int
	Height      int
	SizeBytes   int64
	OriginalURL string
	Derivatives []DerivativeView
}

type DerivativeView struct {
	Label  string
	URL    string
	Width  int
	Height int
}

// Upload 校验→处理→落盘→记库，返回新资产。
func (s *Service) Upload(ctx context.Context, productID int64, data []byte) (*Asset, error) {
	// 1) 准入策略（单文件大小 + 磁盘护栏）。
	if err := s.policy.AllowUpload(int64(len(data))); err != nil {
		return nil, err
	}
	// 2) 单商品张数护栏。
	cnt, err := s.q.CountMediaByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("media: 统计图片失败: %w", err)
	}
	if s.maxPerProduct > 0 && cnt >= int64(s.maxPerProduct) {
		return nil, ErrTooManyPerProduct
	}
	// 3) 校验+处理（去 EXIF、多尺寸 WebP）。
	p, err := Process(data)
	if err != nil {
		return nil, err
	}

	// 4) 落盘：原图 + 派生。内容哈希命名，去重防穿越。
	origPath := path.Join("originals", p.ContentHash+"."+p.Ext)
	if err := s.backend.Put(origPath, p.OriginalBytes); err != nil {
		return nil, err
	}
	for _, d := range p.Derivatives {
		if err := s.backend.Put(derivedPath(p.ContentHash, d.Label), d.Bytes); err != nil {
			return nil, err
		}
	}

	// 5) 记库（事务）。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("media: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	publicID := uuid.Must(uuid.NewV7()).String()
	assetID, err := q.CreateMediaAsset(ctx, sqlcgen.CreateMediaAssetParams{
		PublicID: publicID, ProductID: productID, ContentHash: p.ContentHash, OriginalPath: origPath,
		Mime: p.MIME, Width: int64(p.Width), Height: int64(p.Height), SizeBytes: int64(len(p.OriginalBytes)),
		Position: cnt,
	})
	if err != nil {
		return nil, fmt.Errorf("media: 记录资产失败: %w", err)
	}
	view := &Asset{
		PublicID: publicID, Mime: p.MIME, Width: p.Width, Height: p.Height,
		SizeBytes: int64(len(p.OriginalBytes)), OriginalURL: mediaURL(origPath),
	}
	for _, d := range p.Derivatives {
		dp := derivedPath(p.ContentHash, d.Label)
		if err := q.AddDerivative(ctx, sqlcgen.AddDerivativeParams{
			AssetID: assetID, Label: d.Label, Path: dp, Format: d.Format,
			Width: int64(d.Width), Height: int64(d.Height), SizeBytes: int64(len(d.Bytes)),
		}); err != nil {
			return nil, fmt.Errorf("media: 记录派生失败: %w", err)
		}
		view.Derivatives = append(view.Derivatives, DerivativeView{Label: d.Label, URL: mediaURL(dp), Width: d.Width, Height: d.Height})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("media: 提交失败: %w", err)
	}
	return view, nil
}

// ListByProduct 列出商品的媒体（含派生）。
func (s *Service) ListByProduct(ctx context.Context, productID int64) ([]Asset, error) {
	rows, err := s.q.ListMediaByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("media: 列图片失败: %w", err)
	}
	out := make([]Asset, 0, len(rows))
	for _, r := range rows {
		a := Asset{
			PublicID: r.PublicID, Mime: r.Mime, Width: int(r.Width), Height: int(r.Height),
			SizeBytes: r.SizeBytes, OriginalURL: mediaURL(r.OriginalPath),
		}
		ds, err := s.q.ListDerivativesByAsset(ctx, r.ID)
		if err != nil {
			return nil, fmt.Errorf("media: 列派生失败: %w", err)
		}
		for _, d := range ds {
			a.Derivatives = append(a.Derivatives, DerivativeView{Label: d.Label, URL: mediaURL(d.Path), Width: int(d.Width), Height: int(d.Height)})
		}
		out = append(out, a)
	}
	return out, nil
}

// Delete 删除一张图片：移除文件 + 硬删记录。不存在返回 ErrNotFound。
func (s *Service) Delete(ctx context.Context, publicID string) error {
	m, err := s.q.GetMediaByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("media: 取图片失败: %w", err)
	}
	s.removeFiles(ctx, m.ID, m.OriginalPath)
	if err := s.q.HardDeleteMedia(ctx, m.ID); err != nil {
		return fmt.Errorf("media: 删记录失败: %w", err)
	}
	return nil
}

// CleanupOrphans 清理孤儿媒体（自身软删或所属商品已删）：移除文件 + 硬删记录，返回清理数。
func (s *Service) CleanupOrphans(ctx context.Context) (int, error) {
	orphans, err := s.q.ListOrphanMedia(ctx)
	if err != nil {
		return 0, fmt.Errorf("media: 列孤儿失败: %w", err)
	}
	n := 0
	for _, o := range orphans {
		s.removeFiles(ctx, o.ID, o.OriginalPath)
		if err := s.q.HardDeleteMedia(ctx, o.ID); err != nil {
			return n, fmt.Errorf("media: 删孤儿记录失败: %w", err)
		}
		n++
	}
	return n, nil
}

// removeFiles 删除某资产的原图与全部派生文件（尽力而为，文件缺失不报错）。
func (s *Service) removeFiles(ctx context.Context, assetID int64, originalPath string) {
	_ = s.backend.Remove(originalPath)
	paths, err := s.q.ListDerivativePathsByAsset(ctx, assetID)
	if err != nil {
		return
	}
	for _, p := range paths {
		_ = s.backend.Remove(p)
	}
}

func derivedPath(hash, label string) string { return path.Join("derived", hash+"_"+label+".webp") }

// mediaURL 把相对存储路径映射为可访问 URL（本地后端经 /media/ 提供）。
func mediaURL(relPath string) string { return "/media/" + relPath }
