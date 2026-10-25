// 邮件发送 worker / Email Outbox Worker
// 功能：轮询 outbox、事务外发送、明确失败退避；未知或中断投递保留待核对，日志和错误不含客户/凭证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package mail

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
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

// Run 阻塞运行 worker 直至 ctx 取消。启动将陈留 sending 保留为未知结果，不自动重发。
func (w *Worker) Run(ctx context.Context) {
	if err := w.q.HoldInterruptedEmails(ctx); err != nil {
		w.logger.Warn("邮件 worker 启动核对 sending 失败")
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
	if err := w.q.HoldStaleShipmentEmails(ctx, time.Now().Add(-2*time.Minute).Unix()); err != nil {
		w.logger.Warn("发货通知投递恢复检查失败")
		return
	}
	if err := w.cleanGuestMail(ctx); err != nil {
		w.logger.Warn("验证码邮件清理失败")
		return
	}
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
		rows, err := w.claimEmail(ctx, m.ID, strings.HasPrefix(m.Kind, ShipmentKindPrefix))
		if err != nil || rows == 0 {
			continue
		}
		if strings.HasPrefix(m.Kind, ShipmentKindPrefix) {
			current, err := w.q.ShipmentEmailCurrent(ctx, m.ID)
			if err != nil {
				w.logger.Warn("发货通知版本暂不可确认", "outbox_id", m.ID)
				_ = w.q.MarkEmailRetry(ctx, sqlcgen.MarkEmailRetryParams{Attempts: m.Attempts, NextAttemptAt: time.Now().Add(time.Minute).UTC().Format(tsLayout), LastError: "shipment status unavailable", ID: m.ID})
				continue
			}
			if current == 0 {
				_ = w.q.MarkEmailSuperseded(ctx, m.ID)
				continue
			}
		}
		guest := strings.HasPrefix(m.Kind, guestMailPrefix)
		if guest {
			valid, err := w.guestMailReady(ctx, m.ID)
			if err != nil {
				// A DB failure is not proof of expiry; defer without an SMTP attempt.
				w.logger.Warn("验证码邮件状态暂不可确认", "outbox_id", m.ID)
				_ = w.q.MarkEmailRetry(ctx, sqlcgen.MarkEmailRetryParams{Attempts: m.Attempts, NextAttemptAt: time.Now().Add(time.Minute).UTC().Format(tsLayout), LastError: "verification status unavailable", ID: m.ID})
				continue
			}
			if !valid {
				_ = w.q.MarkEmailSkipped(ctx, sqlcgen.MarkEmailSkippedParams{LastError: "verification expired", ID: m.ID})
				w.clearGuestMail(ctx, m.ID)
				continue
			}
		}
		// 真未配置（D7-A）：标 skipped，不重试、不阻塞。
		if !ready {
			_ = w.q.MarkEmailSkipped(ctx, sqlcgen.MarkEmailSkippedParams{LastError: "SMTP 未配置", ID: m.ID})
			if guest {
				w.clearGuestMail(ctx, m.ID)
			}
			continue
		}
		// —— 脱离 DB 做 SMTP 发送（可能耗时数秒，绝不持连接/事务）——
		sendErr := w.send(cfg, m.ToAddr, m.Subject, m.Body)
		if sendErr == nil {
			if err := w.q.MarkEmailSent(ctx, m.ID); err != nil {
				w.logger.Warn("邮件发送结果保存失败", "outbox_id", m.ID)
				continue
			}
			if guest {
				w.clearGuestMail(ctx, m.ID)
			}
			w.logger.Info("邮件已获 SMTP 接收确认", "outbox_id", m.ID, "order_id", m.OrderID) // 不代表已进收件箱；不记收件人/凭证
			continue
		}
		attempts := m.Attempts + 1
		unknown := errors.Is(sendErr, ErrDeliveryUnknown)
		safeError := "email delivery failed"
		if unknown {
			safeError = "email delivery result unknown"
		}
		if guest {
			safeError = "verification " + safeError
		}
		if unknown {
			_ = w.q.MarkEmailUncertain(ctx, sqlcgen.MarkEmailUncertainParams{Attempts: attempts, LastError: safeError, ID: m.ID})
			w.logger.Warn("邮件投递结果待核对，暂停自动重发", "outbox_id", m.ID)
			continue
		}
		if attempts >= maxAttempts {
			_ = w.q.MarkEmailFailed(ctx, sqlcgen.MarkEmailFailedParams{Attempts: attempts, LastError: safeError, ID: m.ID})
			if guest {
				w.clearGuestMail(ctx, m.ID)
			}
			w.logger.Error("邮件发送最终失败（死信）", "outbox_id", m.ID, "order_id", m.OrderID, "attempts", attempts)
			continue
		}
		next := time.Now().Add(backoff[attempts-1]).UTC().Format(tsLayout)
		_ = w.q.MarkEmailRetry(ctx, sqlcgen.MarkEmailRetryParams{Attempts: attempts, NextAttemptAt: next, LastError: safeError, ID: m.ID})
		w.logger.Warn("邮件发送失败，将重试", "outbox_id", m.ID, "attempts", attempts, "next_at", next)
	}
}

// Persist the claim clock in the same transaction as pending -> sending. This
// lets a later tick hold a lost result-write as unknown instead of blocking all
// tracking edits forever. The SMTP operation still runs after releasing the DB.
func (w *Worker) claimEmail(ctx context.Context, id int64, shipment bool) (int64, error) {
	if !shipment {
		return w.q.MarkEmailSending(ctx, id)
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := w.q.WithTx(tx)
	rows, err := q.MarkEmailSending(ctx, id)
	if err != nil || rows == 0 {
		return rows, err
	}
	if err = q.MarkShipmentClaimTime(ctx, sqlcgen.MarkShipmentClaimTimeParams{SendingAt: time.Now().Unix(), OutboxID: id}); err != nil {
		return 0, err
	}
	return rows, tx.Commit()
}
