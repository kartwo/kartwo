// 订单库存预留生命周期 / Order Reservation Lifecycle
// 功能：未付款订单到期释放库存，付款时把预留转成实际扣减；事务内幂等执行
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-09 11:24:00
package order

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const ReservationTTL = 30 * time.Minute
const reservationTimeLayout = "2006-01-02T15:04:05.000Z"

func reservationTimestamp(t time.Time) string { return t.UTC().Format(reservationTimeLayout) }

// ExpirePendingTx 在订单仍未付且到期时取消订单并释放对应预留。
// 调用方持有事务；与付款事务串行后，恰好只有一种状态迁移成功。
func ExpirePendingTx(ctx context.Context, tx *sql.Tx, orderID int64, now time.Time) (bool, error) {
	var status, until string
	if err := tx.QueryRowContext(ctx, `SELECT status,reservation_expires_at FROM "order" WHERE id=?`, orderID).Scan(&status, &until); err != nil {
		return false, fmt.Errorf("order: 读取预留期限: %w", err)
	}
	if status != "pending" || until == "" || until > reservationTimestamp(now) {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE "order" SET status='cancelled',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND status='pending' AND reservation_expires_at<=?`, orderID, reservationTimestamp(now))
	if err != nil {
		return false, fmt.Errorf("order: 取消到期订单: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		return false, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT variant_id,quantity FROM order_item WHERE order_id=?`, orderID)
	if err != nil {
		return false, fmt.Errorf("order: 读取到期订单行: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type line struct{ variantID, quantity int64 }
	var lines []line
	for rows.Next() {
		var item line
		if err := rows.Scan(&item.variantID, &item.quantity); err != nil {
			_ = rows.Close()
			return false, err
		}
		lines = append(lines, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	_ = rows.Close()
	for _, item := range lines {
		result, err := tx.ExecContext(ctx, `UPDATE inventory SET reserved=reserved-?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE variant_id=? AND reserved>=?`, item.quantity, item.variantID, item.quantity)
		if err != nil {
			return false, fmt.Errorf("order: 释放预留库存: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if n != 1 {
			return false, fmt.Errorf("order: 释放预留库存失败：变体 %d 预留不足", item.variantID)
		}
	}
	return true, nil
}

// FinalizePaidReservationTx 把已付款订单的预留转为实际库存扣减。
func FinalizePaidReservationTx(ctx context.Context, tx *sql.Tx, orderID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT variant_id,quantity FROM order_item WHERE order_id=?`, orderID)
	if err != nil {
		return fmt.Errorf("order: 读取付款订单行: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type line struct{ variantID, quantity int64 }
	var lines []line
	for rows.Next() {
		var item line
		if err := rows.Scan(&item.variantID, &item.quantity); err != nil {
			_ = rows.Close()
			return err
		}
		lines = append(lines, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for _, item := range lines {
		result, err := tx.ExecContext(ctx, `UPDATE inventory SET quantity=quantity-?,reserved=reserved-?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE variant_id=? AND quantity>=? AND reserved>=?`, item.quantity, item.quantity, item.variantID, item.quantity, item.quantity)
		if err != nil {
			return fmt.Errorf("order: 结算库存: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("order: 结算库存失败：变体 %d 数量不一致", item.variantID)
		}
	}
	return nil
}

// ExpireDue 批量取消到期未付订单。每笔订单单独事务，失败不吞掉且可重试。
func (s *Service) ExpireDue(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM "order" WHERE status='pending' AND reservation_expires_at<>'' AND reservation_expires_at<=? ORDER BY reservation_expires_at LIMIT 100`, reservationTimestamp(now))
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	_ = rows.Close()
	count := 0
	for _, id := range ids {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return count, err
		}
		changed, err := ExpirePendingTx(ctx, tx, id, now)
		if err != nil {
			_ = tx.Rollback()
			return count, err
		}
		if err := tx.Commit(); err != nil {
			return count, err
		}
		if changed {
			count++
		}
	}
	return count, nil
}

func (s *Service) RunReservationExpiry(ctx context.Context, logger *slog.Logger) {
	run := func() {
		for {
			count, err := s.ExpireDue(ctx, time.Now())
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("到期订单库存释放失败", "err", err)
				}
				return
			}
			if count > 0 {
				logger.Info("到期未付订单已取消并释放库存", "orders", count)
			}
			if count < 100 {
				return
			}
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
