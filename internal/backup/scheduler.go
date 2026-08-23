// 本地备份调度器 / Local Backup Scheduler
// 功能：服务启动即创建全量备份，并按固定周期继续执行与清理旧备份
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 10:30:00
package backup

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler 在服务进程内管理本地自动备份，不持久化任务状态。
type Scheduler struct {
	exporter  *Exporter
	interval  time.Duration
	retention int
	logger    *slog.Logger
}

// NewScheduler 构造本地备份调度器。配置加载层已保证 interval/retention 合法。
func NewScheduler(exporter *Exporter, interval time.Duration, retention int, logger *slog.Logger) *Scheduler {
	return &Scheduler{exporter: exporter, interval: interval, retention: retention, logger: logger}
}

// Run 阻塞运行直至 ctx 取消。启动立即执行一次，确保新部署无需等待首个周期。
func (s *Scheduler) Run(ctx context.Context) {
	s.logger.Info("本地备份调度器启动", "interval", s.interval.String(), "retention", s.retention)
	s.tick(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("本地备份调度器停止")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	path, _, err := s.exporter.CreatePersistent(ctx)
	if err != nil {
		s.logger.Warn("本地自动备份失败", "err", err)
		return
	}
	if err := PrunePersistent(s.exporter.dataDir, s.retention); err != nil {
		s.logger.Warn("清理旧自动备份失败", "err", err)
		return
	}
	s.logger.Info("本地自动备份完成", "path", path)
}
