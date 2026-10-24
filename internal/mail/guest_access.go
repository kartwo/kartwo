// 验证码邮件保护 / Verification Email Protection
// 功能：发送前校验一次性挑战，过期/作废邮件不发送，发送后清除验证码正文
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package mail

import (
	"context"
	"time"
)

const guestMailPrefix = "guest_access:"

// Cleanup happens even when SMTP is locked. Only transient code messages are
// scrubbed; ordinary order notifications retain their established lifecycle.
func (w *Worker) cleanGuestMail(ctx context.Context) error {
	_, err := w.db.ExecContext(ctx, `UPDATE email_outbox SET body='',status=CASE WHEN status IN ('pending','sending') THEN 'skipped' ELSE status END WHERE kind GLOB 'guest_access:*' AND body<>'' AND (status IN ('sent','failed','skipped') OR NOT EXISTS (SELECT 1 FROM order_guest_challenge c WHERE c.outbox_id=email_outbox.id AND c.order_id=email_outbox.order_id AND email_outbox.kind='guest_access:'||c.public_id AND c.expires_at>? AND c.attempts<5 AND c.consumed_at=0))`, time.Now().Unix())
	return err
}
func (w *Worker) guestMailReady(ctx context.Context, id int64) (bool, error) {
	var ready bool
	err := w.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM order_guest_challenge c JOIN email_outbox e ON e.id=c.outbox_id AND e.order_id=c.order_id AND e.kind='guest_access:'||c.public_id WHERE c.outbox_id=? AND c.expires_at>? AND c.attempts<5 AND c.consumed_at=0)`, id, time.Now().Unix()).Scan(&ready)
	return ready, err
}
func (w *Worker) clearGuestMail(ctx context.Context, id int64) {
	_, _ = w.db.ExecContext(ctx, `UPDATE email_outbox SET body='' WHERE id=? AND kind GLOB 'guest_access:*'`, id)
}
