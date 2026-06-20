// 收款密钥内存缓存 / In-Memory Payment Key Cache
// 功能：登录解锁(KEK)时解密收款密钥入内存、退出即销毁、改密钥时立即重载；Webhook/建会话从此取密钥（绝不落盘）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/kartwo/kartwo/internal/settings"
)

// KeyCache 进程级持有【已解密】的收款密钥。
// 生命周期严格绑定 KEK 金库：Unlock=登录解锁时载入；Lock=登出时销毁；改密钥后立即 Unlock 重载。
// 锁定（未解锁或无密钥）时，依赖它的能力一律不可用——Webhook 据此返回非 2xx 交网关重投，绝不放行。
type KeyCache struct {
	settings *settings.Service

	mu          sync.RWMutex
	mode        string
	publishable string
	secret      string // sk_*
	webhook     string // whsec_*
}

// NewKeyCache 构造空（锁定）缓存。
func NewKeyCache(s *settings.Service) *KeyCache { return &KeyCache{settings: s} }

// Unlock 用 KEK 解密收款密钥载入内存。登录成功与改密钥保存后调用。
// 未配置的项留空（视作锁定/未配置，效果同未解锁）。
func (c *KeyCache) Unlock(ctx context.Context, kek []byte) error {
	mode, _ := c.settings.Get(ctx, KeyStripeMode)
	if strings.TrimSpace(mode) == "" {
		mode = "test" // 默认沙箱
	}
	pub, _ := c.settings.Get(ctx, KeyStripePublishable)

	secret, err := c.settings.GetEncrypted(ctx, KeyStripeSecret, kek)
	if err != nil && !errors.Is(err, settings.ErrNotFound) {
		return err
	}
	webhook, err := c.settings.GetEncrypted(ctx, KeyStripeWebhookSecret, kek)
	if err != nil && !errors.Is(err, settings.ErrNotFound) {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = mode
	c.publishable = pub
	c.secret = string(secret)
	c.webhook = string(webhook)
	return nil
}

// Lock 销毁内存中的密钥（登出时调用）。
// 注：Go 字符串不可变，无法原地清零；此处丢弃引用，等同金库失效。
func (c *KeyCache) Lock() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode, c.publishable, c.secret, c.webhook = "", "", "", ""
}

// secretKey 返回 sk_*；未解锁/未配置返回 false。
func (c *KeyCache) secretKey() (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.secret == "" {
		return "", false
	}
	return c.secret, true
}

// webhookSecret 返回 whsec_*；未解锁/未配置返回 false。
func (c *KeyCache) webhookSecret() (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.webhook == "" {
		return "", false
	}
	return c.webhook, true
}

// Status 返回不含密钥明文的状态快照，供后台收款页展示。
func (c *KeyCache) Status() (mode, publishable string, hasSecret, hasWebhook bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode, c.publishable, c.secret != "", c.webhook != ""
}
