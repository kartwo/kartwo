// 退款结果查询 / Refund Reconciliation Worker
// 功能：定期只读核对已知网关退款编号，未知结果到期转待核对，不自动发送退款
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 23:50:00
package payment

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const refundCheckDelay = 5 * time.Minute

// RunRefundReconciliation resumes saved pending results after restart. Money-
// moving POST requests remain exclusive to an explicit merchant action.
func (s *Service) RunRefundReconciliation(ctx context.Context, logger *slog.Logger) {
	run := func() {
		if err := s.pollRefunds(ctx); err != nil && ctx.Err() == nil {
			// Provider errors are intentionally not logged (they may contain keys
			// or customer data). The saved request retains a safe reason code.
			logger.Warn("退款状态查询本轮未完成，将在后续轮次继续")
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
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

type refundCheckCandidate struct {
	id, orderID, gatewayRef, updated string
	firstSent                        int64
}

// dueRefundChecks closes the cursor before any update or network I/O;
// SQLite intentionally uses only one connection.
func (s *Service) dueRefundChecks(ctx context.Context) ([]refundCheckCandidate, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-refundCheckDelay).Format("2006-01-02T15:04:05.000Z")
	rows, err := s.db.QueryContext(ctx, `SELECT r.public_id,o.public_id,r.gateway_ref,r.first_sent_at,r.updated_at
        FROM refund_request r JOIN "order" o ON o.id=r.order_id
        WHERE r.state='pending' AND r.updated_at<=? AND r.lease_until<=?
        ORDER BY r.updated_at,r.id LIMIT 10`, cutoff, now.Unix())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var batch []refundCheckCandidate
	for rows.Next() {
		var c refundCheckCandidate
		if err := rows.Scan(&c.id, &c.orderID, &c.gatewayRef, &c.firstSent, &c.updated); err != nil {
			return nil, err
		}
		batch = append(batch, c)
	}
	return batch, rows.Err()
}

func (s *Service) pollRefunds(ctx context.Context) error {
	batch, err := s.dueRefundChecks(ctx)
	if err != nil {
		return err
	}
	for _, c := range batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Compare the observed timestamp so parallel instances do not query the
		// same row in one pass. Rotate even locked/config-changed requests to
		// prevent a permanently blocked row starving the rest of the queue.
		result, err := s.db.ExecContext(ctx, `UPDATE refund_request SET updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND state='pending' AND updated_at=? AND lease_until<=?`, c.id, c.updated, time.Now().Unix())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		if c.gatewayRef == "" {
			if c.firstSent > 0 && time.Now().Unix()-c.firstSent > int64((15*time.Minute)/time.Second) {
				if _, err := s.db.ExecContext(ctx, `UPDATE refund_request SET state='review',reason='unknown_result' WHERE public_id=? AND state='pending' AND gateway_ref='' AND lease_until<=?`, c.id, time.Now().Unix()); err != nil {
					return err
				}
			}
			continue // Never call RetryRefund without a known gateway refund ID.
		}
		_, err = s.RetryRefund(ctx, c.orderID, c.id, c.gatewayRef)
		if err == nil || errors.Is(err, ErrRefundBusy) {
			continue
		}
		reason := "gateway_unconfirmed"
		switch {
		case errors.Is(err, ErrLocked):
			reason = "credentials_locked"
		case errors.Is(err, ErrPaymentConfigChanged):
			reason = "configuration_changed"
		}
		if _, saveErr := s.db.ExecContext(ctx, `UPDATE refund_request SET reason=? WHERE public_id=? AND state='pending' AND lease_until<=?`, reason, c.id, time.Now().Unix()); saveErr != nil {
			return saveErr
		}
	}
	return nil
}
