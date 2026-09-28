// 留言隐私清理 / Contact Privacy Worker
// 功能：按后台期限每日匿名化到期留言与原始 IP，不删除工单元数据
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 14:25:00
package contact

import (
	"context"
	"log/slog"
	"time"
)

func (s *Service) RunRetention(ctx context.Context, logger *slog.Logger) {
	run := func() {
		if err := s.AnonymizeDue(ctx, time.Now()); err != nil && ctx.Err() == nil {
			logger.Error("客服留言匿名化失败", "err", err)
		}
	}
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
