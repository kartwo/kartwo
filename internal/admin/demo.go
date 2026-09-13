// 公开演示生命周期 / Public Demo Lifecycle
// 功能：会话级临时商品配额、对象归属校验、重置与过期资源清理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-13 11:20:00
package admin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var (
	ErrDemoForbidden = fmt.Errorf("admin: 公开演示无权修改该资源")
	ErrDemoQuota     = fmt.Errorf("admin: 本次演示最多可创建 3 个临时商品")
)

func (h *HTTP) demoOwnsProduct(ctx context.Context, token, publicID string) (bool, error) {
	n, err := h.svc.q.IsDemoProductOwnedBySession(ctx, sqlcgen.IsDemoProductOwnedBySessionParams{PublicID: publicID, SessionToken: token})
	return n != 0, err
}

func (h *HTTP) demoCanViewProduct(ctx context.Context, token, publicID string) (bool, error) {
	n, err := h.svc.q.CanDemoViewProduct(ctx, sqlcgen.CanDemoViewProductParams{PublicID: publicID, SessionToken: token})
	return n != 0, err
}

func (h *HTTP) demoOwnsVariant(ctx context.Context, token, publicID string) (bool, error) {
	n, err := h.svc.q.IsDemoVariantOwnedBySession(ctx, sqlcgen.IsDemoVariantOwnedBySessionParams{PublicID: publicID, SessionToken: token})
	return n != 0, err
}

func (h *HTTP) demoOwnsMedia(ctx context.Context, token, publicID string) (bool, error) {
	n, err := h.svc.q.IsDemoMediaOwnedBySession(ctx, sqlcgen.IsDemoMediaOwnedBySessionParams{PublicID: publicID, SessionToken: token})
	return n != 0, err
}

func (h *HTTP) claimDemoProduct(ctx context.Context, ac *AuthContext, publicID string) error {
	count, err := h.svc.q.CountDemoProductsBySession(ctx, ac.SessionToken)
	if err != nil {
		return err
	}
	if count >= int64(h.demo.MaxProducts) {
		return ErrDemoQuota
	}
	productID, err := h.cat.ProductIDByPublicID(ctx, publicID)
	if err != nil {
		return err
	}
	for slot := int64(1); slot <= int64(h.demo.MaxProducts); slot++ {
		err = h.svc.q.ClaimDemoProduct(ctx, sqlcgen.ClaimDemoProductParams{ProductID: productID, SessionToken: ac.SessionToken, Slot: slot, ExpiresAt: ac.ExpiresAt.UTC().Format(timeLayout)})
		if err == nil {
			return nil
		}
	}
	return ErrDemoQuota
}

func (h *HTTP) removeDemoProduct(ctx context.Context, productID int64, publicID string) error {
	if err := h.cat.DeleteProduct(ctx, publicID); err != nil && err != catalog.ErrNotFound {
		return err
	}
	if _, err := h.media.CleanupOrphans(ctx); err != nil {
		return err
	}
	if err := h.svc.q.HardDeleteProduct(ctx, productID); err != nil {
		return err
	}
	return nil
}

func (h *HTTP) cleanupDemoSession(ctx context.Context, token string) (int, error) {
	rows, err := h.svc.q.ListDemoProductsBySession(ctx, token)
	if err != nil {
		return 0, err
	}
	for i, row := range rows {
		if err := h.removeDemoProduct(ctx, row.ID, row.PublicID); err != nil {
			return i, err
		}
	}
	return len(rows), nil
}

// CleanupExpiredDemoProducts 清除过期会话创建的商品及其图片文件。
func (h *HTTP) CleanupExpiredDemoProducts(ctx context.Context) (int, error) {
	rows, err := h.svc.q.ListExpiredDemoProducts(ctx, time.Now().UTC().Format(timeLayout))
	if err != nil {
		return 0, err
	}
	for i, row := range rows {
		if err := h.removeDemoProduct(ctx, row.ID, row.PublicID); err != nil {
			return i, err
		}
	}
	if err := h.svc.q.DeleteExpiredSessions(ctx, time.Now().UTC().Format(timeLayout)); err != nil && err != sql.ErrNoRows {
		return len(rows), err
	}
	return len(rows), nil
}

// RunDemoCleanup 周期执行公开演示垃圾回收，直到服务上下文取消。
func (h *HTTP) RunDemoCleanup(ctx context.Context, logger *slog.Logger) {
	if !h.demo.Enabled {
		return
	}
	ticker := time.NewTicker(h.demo.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := h.CleanupExpiredDemoProducts(ctx)
			if err != nil {
				logger.Error("公开演示资源清理失败", "error", err)
			} else if n > 0 {
				logger.Info("公开演示资源已清理", "products", n)
			}
		}
	}
}

func (h *HTTP) resetDemo(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	if ac.Role != "demo" {
		writeErr(w, http.StatusForbidden, "仅公开演示会话可重置")
		return
	}
	n, err := h.cleanupDemoSession(r.Context(), ac.SessionToken)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "重置失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed_products": n})
}
