// 邮件发送 worker / Email Outbox Worker
// 功能：ticker 轮询 outbox（D8 20s）→ 认领 pending → 脱离 DB 做 SMTP 发送 → 回写 sent/failed/skipped；重试指数退避(D6)
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package mail

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// tsLayout 匹配库内 strftime('%Y-%m-%dT%H:%M:%fZ') 的 UTC 形态（词法可比）。
const tsLayout = "2006-01-02T15:04:05.000Z"

// maxAttempts 达此次数仍失败即标 failed 死信（D6：上限 5 次）。
const maxAttempts = 5

// batchSize 单次 tick 认领上限。
const batchSize = 10

// backoff 第 n 次失败后的重试间隔（n=1..4；第 5 次失败即死信，不再退避）。D6：1m/5m/30m/2h/6h。
var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}

// Worker 后台轮询 outbox 发送邮件。绝不在持有 DB 连接/事务时做 SMTP 网络 I/O（守单连接纪律）。
type Worker struct {
	db       *sql.DB
	q        *sqlcgen.Queries
	cache    *Cache
	logger   *slog.Logger
	interval time.Duration
	send     func(Config, string, string, string) error // 可注入（测试用），默认 Send
}

// NewWorker 构造 worker。interval<=0 用默认 20s（D8）。
func NewWorker(db *sql.DB, cache *Cache, logger *slog.Logger, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = 20 * time.Second
	}
	return &Worker{db: db, q: sqlcgen.New(db), cache: cache, logger: logger, interval: interval, send: Send}
}

// Run 阻塞运行 worker 直至 ctx 取消（接 main 优雅关停）。启动先恢复陈留 sending。
func (w *Worker) Run(ctx context.Context) {
	if err := w.q.ResetStaleSending(ctx); err != nil {
		w.logger.Warn("邮件 worker 启动恢复 sending 失败", "err", err)
	}
	w.logger.Info("邮件发送 worker 启动", "interval", w.interval.String())
	t := time.NewTicker(w.interval)
	defer t.Stop()
	w.tick(ctx) // 启动即跑一轮，不等第一个 tick
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("邮件发送 worker 停止")
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

// tick 处理一批到期邮件。发送在事务/连接之外进行。
func (w *Worker) tick(ctx context.Context) {
	cfg, ready := w.cache.Config()
	// 已配置但金库锁定（KEK 未解锁）：全部留 pending、本轮不认领，待登录解锁后补发（D2）。
	if !ready && w.cache.Configured(ctx) {
		return
	}
	now := time.Now().UTC().Format(tsLayout)
	claimed, err := w.q.ClaimDueEmails(ctx, sqlcgen.ClaimDueEmailsParams{NextAttemptAt: now, Limit: batchSize})
	if err != nil {
		w.logger.Warn("邮件 worker 认领失败", "err", err)
		return
	}
	for _, m := range claimed {
		// 快速事务：pending->sending 认领（rows=1 确认）。
		rows, err := w.q.MarkEmailSending(ctx, m.ID)
		if err != nil || rows == 0 {
			continue
		}
		// 真未配置（D7-A）：标 skipped，不重试、不阻塞。
		if !ready {
			_ = w.q.MarkEmailSkipped(ctx, sqlcgen.MarkEmailSkippedParams{LastError: "SMTP 未配置", ID: m.ID})
			continue
		}
		// —— 脱离 DB 做 SMTP 发送（可能耗时数秒，绝不持连接/事务）——
		sendErr := w.send(cfg, m.ToAddr, m.Subject, m.Body)
		if sendErr == nil {
			_ = w.q.MarkEmailSent(ctx, m.ID)
			w.logger.Info("确认信已发送", "outbox_id", m.ID, "order_id", m.OrderID) // 不记收件人/凭证
			continue
		}
		attempts := m.Attempts + 1
		if attempts >= maxAttempts {
			_ = w.q.MarkEmailFailed(ctx, sqlcgen.MarkEmailFailedParams{Attempts: attempts, LastError: sendErr.Error(), ID: m.ID})
			w.logger.Error("确认信发送最终失败（死信）", "outbox_id", m.ID, "order_id", m.OrderID, "attempts", attempts, "err", sendErr)
			continue
		}
		next := time.Now().Add(backoff[attempts-1]).UTC().Format(tsLayout)
		_ = w.q.MarkEmailRetry(ctx, sqlcgen.MarkEmailRetryParams{Attempts: attempts, NextAttemptAt: next, LastError: sendErr.Error(), ID: m.ID})
		w.logger.Warn("确认信发送失败，将重试", "outbox_id", m.ID, "attempts", attempts, "next_at", next, "err", sendErr)
	}
}
