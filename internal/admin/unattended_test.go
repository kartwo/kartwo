// 无人值守业务密钥测试 / Unattended Business Key Tests
// 功能：验证退出、重启、主密码轮换和迁店后重新配对的收款恢复
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-09 12:00:00
package admin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/settings"
)

func TestUnattendedKeysSurviveLogoutRestartAndPasswordChange(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	keyPath := filepath.Join(t.TempDir(), "shop.unlock-key")
	svc.SetRuntimeKeyPath(keyPath)
	settingSvc := settings.New(svc.db)
	keys := payment.NewKeyCache(settingSvc)
	mailKeys := mail.NewCache(settingSvc)
	svc.SetPaymentKeys(keys)
	svc.SetMailKeys(mailKeys)
	if err := svc.Initialize(ctx, "owner", "old-password"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("本机钥匙权限不正确: info=%v err=%v", info, err)
	}
	session, err := svc.Login(ctx, "owner", "old-password")
	if err != nil {
		t.Fatal(err)
	}
	kek, ok := svc.Key(session.Token)
	if !ok {
		t.Fatal("店主会话无 KEK")
	}
	if err := settingSvc.SetEncrypted(ctx, payment.KeyStripeSecret, []byte("fixture-payment-secret"), kek); err != nil {
		t.Fatal(err)
	}
	if err := settingSvc.SetEncrypted(ctx, mail.KeySMTPPassword, []byte("mail-pass"), kek); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReloadKeys(ctx, kek); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReloadMailKeys(ctx, kek); err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if !keys.Status().StripeHasSecret {
		t.Fatal("退出后台后应继续收款")
	}
	if cfg, _ := mailKeys.Config(); cfg.Password != "mail-pass" {
		t.Fatal("退出后台后邮件配置应继续可用")
	}

	restarted := New(svc.db)
	restarted.SetRuntimeKeyPath(keyPath)
	restartedKeys := payment.NewKeyCache(settingSvc)
	restartedMail := mail.NewCache(settingSvc)
	restarted.SetPaymentKeys(restartedKeys)
	restarted.SetMailKeys(restartedMail)
	if restored, err := restarted.AutoUnlockBusinessKeys(ctx); err != nil || !restored {
		t.Fatalf("重启应自动解锁: restored=%v err=%v", restored, err)
	}
	if !restartedKeys.Status().StripeHasSecret {
		t.Fatal("重启后应自动恢复收款")
	}
	if cfg, _ := restartedMail.Config(); cfg.Password != "mail-pass" {
		t.Fatal("重启后应自动恢复邮件密码")
	}

	session, err = restarted.Login(ctx, "owner", "old-password")
	if err != nil {
		t.Fatal(err)
	}
	ac, err := restarted.Authenticate(ctx, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := restarted.UpdateOwnerCredentials(ctx, ac.AdminID, "owner", "old-password", "new-password"); err != nil || !changed {
		t.Fatalf("改主密码失败: changed=%v err=%v", changed, err)
	}
	if !restartedKeys.Status().StripeHasSecret {
		t.Fatal("改主密码后应继续收款")
	}
	if _, err := restarted.Login(ctx, "owner", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("旧密码仍可登录: %v", err)
	}
	rotated := New(svc.db)
	rotated.SetRuntimeKeyPath(keyPath)
	rotatedKeys := payment.NewKeyCache(settingSvc)
	rotated.SetPaymentKeys(rotatedKeys)
	if restored, err := rotated.AutoUnlockBusinessKeys(ctx); err != nil || !restored || !rotatedKeys.Status().StripeHasSecret {
		t.Fatalf("改密后重启仍须自动恢复收款: restored=%v err=%v", restored, err)
	}

	// 迁店时只搬数据文件夹，旧机器钥匙不存在；店主登录后重新配对。
	newPath := filepath.Join(t.TempDir(), "moved.unlock-key")
	moved := New(svc.db)
	moved.SetRuntimeKeyPath(newPath)
	movedKeys := payment.NewKeyCache(settingSvc)
	moved.SetPaymentKeys(movedKeys)
	if restored, err := moved.AutoUnlockBusinessKeys(ctx); err == nil || restored {
		t.Fatalf("缺旧机器钥匙不可静默解锁: restored=%v err=%v", restored, err)
	}
	if _, err := moved.Login(ctx, "owner", "new-password"); err != nil {
		t.Fatalf("店主应可登录重新配对: %v", err)
	}
	if restored, err := moved.AutoUnlockBusinessKeys(ctx); err != nil || !restored || !movedKeys.Status().StripeHasSecret {
		t.Fatalf("重新配对后应恢复: restored=%v err=%v", restored, err)
	}
}
