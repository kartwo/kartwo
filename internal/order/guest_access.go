// 游客订单凭证 / Guest Order Credentials
// 功能：订单范围内的随机浏览器凭证、邮箱验证、持久限额和一次性兑换；原子绑定下单事务
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package order

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

const GuestAccessTTL = 30 * 24 * time.Hour
const GuestChallengeTTL = 10 * time.Minute

var ErrGuestVerification = errors.New("order: verification code is invalid or expired")

func NewGuestToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func ValidGuestToken(token string) bool { return guestTokenHash(token) != "" }

func guestTokenHash(token string) string {
	if len(token) != 43 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func guestCodeHash(browserHash, code string) string {
	sum := sha256.Sum256([]byte("kartwo-order-code:" + browserHash + ":" + code))
	return hex.EncodeToString(sum[:])
}

// CheckoutForGuest mints the browser credential before checkout, and persists its
// digest inside the same transaction as the order and inventory reservation.
func (s *Service) CheckoutForGuest(ctx context.Context, cartID int64, info CheckoutInfo) (string, string, error) {
	token, err := NewGuestToken()
	if err != nil {
		return "", "", err
	}
	id, err := s.checkout(ctx, cartID, info, guestTokenHash(token))
	if err != nil {
		return "", "", err
	}
	return id, token, nil
}
func (s *Service) GuestAuthorized(ctx context.Context, publicID, token string) (bool, error) {
	digest := guestTokenHash(token)
	if digest == "" {
		return false, nil
	}
	var allowed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM order_guest_access a JOIN "order" o ON o.id=a.order_id WHERE o.public_id=? AND a.token_hash=? AND a.expires_at>?)`, publicID, digest, time.Now().Unix()).Scan(&allowed)
	return allowed, err
}
func (s *Service) RevokeGuestAccess(ctx context.Context, publicID, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM order_guest_access WHERE token_hash=? AND order_id=(SELECT id FROM "order" WHERE public_id=?)`, guestTokenHash(token), publicID)
	return err
}

// RequestGuestAccess intentionally gives no existence, address-match or quota
// signal. The caller only invokes this when mail is available. No SMTP call is
// made here: the challenge and its expiring outbox message commit atomically.
func (s *Service) RequestGuestAccess(ctx context.Context, publicID, email, browser string) error {
	browserHash := guestTokenHash(browser)
	if browserHash == "" || len(email) > 320 {
		return nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%08d", n.Int64())
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var orderID int64
	var recipient string
	err = tx.QueryRowContext(ctx, `SELECT id,email FROM "order" WHERE public_id=?`, publicID).Scan(&orderID, &recipient)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !validOrderEmail(recipient) || !strings.EqualFold(strings.TrimSpace(recipient), strings.TrimSpace(email)) {
		return nil
	}
	var count int
	var last int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(requested_at),0) FROM order_guest_challenge WHERE order_id=? AND requested_at>?`, orderID, now-3600).Scan(&count, &last); err != nil {
		return err
	}
	if count >= 3 || last > now-60 {
		return nil
	}
	// A new request replaces only this browser's previous challenge; other devices
	// keep their own challenge. All attempts count toward the per-order send limit.
	if _, err = tx.ExecContext(ctx, `UPDATE order_guest_challenge SET consumed_at=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE order_id=? AND browser_hash=? AND consumed_at=0`, now, orderID, browserHash); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE email_outbox SET body='',status=CASE WHEN status IN ('pending','sending') THEN 'skipped' ELSE status END WHERE id IN (SELECT outbox_id FROM order_guest_challenge WHERE order_id=? AND browser_hash=? AND consumed_at<>0)`, orderID, browserHash); err != nil {
		return err
	}
	body := fmt.Sprintf("Your order access code is: %s\n\nOrder: %s\n\nReturn to the store page where you requested this code. It expires in 10 minutes and can be used once. Do not share it. If you did not request this code, ignore this email.\n", code, publicID)
	result, err := tx.ExecContext(ctx, `INSERT INTO email_outbox(order_id,kind,to_addr,subject,body) VALUES(?,?,?,?,?)`, orderID, "guest_access:"+id, strings.TrimSpace(recipient), "Your order access code", body)
	if err != nil {
		return err
	}
	outboxID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO order_guest_challenge(public_id,order_id,browser_hash,code_hash,outbox_id,expires_at,requested_at) VALUES(?,?,?,?,?,?,?)`, id, orderID, browserHash, guestCodeHash(browserHash, code), outboxID, now+int64(GuestChallengeTTL/time.Second), now); err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyGuestAccess consumes a challenge and issues a fresh independent access
// token in one transaction. Neither a guessed order ID nor an email address is
// sufficient to obtain order data or to create a payment session.
func (s *Service) VerifyGuestAccess(ctx context.Context, publicID, browser, code string) (string, error) {
	browserHash := guestTokenHash(browser)
	if browserHash == "" {
		return "", ErrGuestVerification
	}
	token, err := NewGuestToken()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var id, digest string
	var orderID, outboxID int64
	var attempts int
	err = tx.QueryRowContext(ctx, `SELECT c.public_id,c.order_id,c.outbox_id,c.code_hash,c.attempts FROM order_guest_challenge c JOIN "order" o ON o.id=c.order_id WHERE o.public_id=? AND c.browser_hash=? AND c.consumed_at=0 AND c.expires_at>? AND c.attempts<5 ORDER BY c.requested_at DESC,c.public_id DESC LIMIT 1`, publicID, browserHash, now).Scan(&id, &orderID, &outboxID, &digest, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrGuestVerification
	}
	if err != nil {
		return "", err
	}
	candidate := guestCodeHash(browserHash, strings.TrimSpace(code))
	success := subtle.ConstantTimeCompare([]byte(digest), []byte(candidate)) == 1
	consumed := int64(0)
	if success || attempts+1 >= 5 {
		consumed = now
	}
	if _, err = tx.ExecContext(ctx, `UPDATE order_guest_challenge SET attempts=attempts+1,consumed_at=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=?`, consumed, id); err != nil {
		return "", err
	}
	if consumed != 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE email_outbox SET body='',status=CASE WHEN status IN ('pending','sending') THEN 'skipped' ELSE status END WHERE id=?`, outboxID); err != nil {
			return "", err
		}
	}
	if success {
		if _, err = tx.ExecContext(ctx, `INSERT INTO order_guest_access(token_hash,order_id,expires_at) VALUES(?,?,?)`, guestTokenHash(token), orderID, now+int64(GuestAccessTTL/time.Second)); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	if !success {
		return "", ErrGuestVerification
	}
	return token, nil
}

// RevokeAllGuestAccess is a merchant action: revoke every current device and
// outstanding code for this order while retaining rate-limit evidence.
func (s *Service) RevokeAllGuestAccess(ctx context.Context, publicID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM "order" WHERE public_id=?`, publicID).Scan(&id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM order_guest_access WHERE order_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE order_guest_challenge SET consumed_at=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE order_id=? AND consumed_at=0`, time.Now().Unix(), id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE email_outbox SET body='',status=CASE WHEN status IN ('pending','sending') THEN 'skipped' ELSE status END WHERE id IN(SELECT outbox_id FROM order_guest_challenge WHERE order_id=?)`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CleanGuestAccess removes expired grants and keeps one day of challenge quota
// evidence. Bodies are erased before deleting their expiration/consumption data.
func (s *Service) CleanGuestAccess(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM order_guest_access WHERE expires_at<=?`, now.Unix()); err != nil {
		return err
	}
	cutoff := now.Add(-24 * time.Hour).Unix()
	if _, err = tx.ExecContext(ctx, `UPDATE email_outbox SET body='',status=CASE WHEN status IN ('pending','sending') THEN 'skipped' ELSE status END WHERE id IN (SELECT outbox_id FROM order_guest_challenge WHERE requested_at<?)`, cutoff); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM order_guest_challenge WHERE requested_at<?`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}
