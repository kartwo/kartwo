// 收款密钥内存缓存 / In-Memory Payment Key Cache
// 功能：默认从加密库(KEK)解密载入收款密钥(登录解锁/登出销毁/改密钥重载)；每通道可选 env 覆盖旁路(env>加密库,覆盖非双写)
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-23 00:20:52
package payment

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/kartwo/kartwo/internal/settings"
)

// 环境变量名（自部署高级运维/本地联调可选覆盖；普通商家仍走收款页加密库，见北极星）。
const (
	envStripeSecret      = "STRIPE_SECRET_KEY"      //nolint:gosec // 环境变量「名」，非凭证本身
	envStripeWebhook     = "STRIPE_WEBHOOK_SECRET"  //nolint:gosec // 环境变量「名」，非凭证本身
	envStripePublishable = "STRIPE_PUBLISHABLE_KEY" //nolint:gosec // 环境变量「名」，非凭证本身
	envStripeMode        = "STRIPE_MODE"
	envPayPalClientID    = "PAYPAL_CLIENT_ID"
	envPayPalSecret      = "PAYPAL_SECRET" //nolint:gosec // 环境变量「名」，非凭证本身
	envPayPalMode        = "PAYPAL_MODE"
)

// CacheStatus 是不含密钥明文的状态快照，供后台收款页/诊断展示。
type CacheStatus struct {
	StripeSource      string // env | db：Stripe 密钥来源
	StripeMode        string
	StripePublishable string
	StripeHasSecret   bool
	StripeHasWebhook  bool
	PayPalSource      string // env | db
	PayPalMode        string
	PayPalClientID    string
	PayPalHasSecret   bool
}

// stripeEnv 是启动时解析的 Stripe 环境变量覆盖；active=false 表示走加密库默认路径。
type stripeEnv struct {
	active      bool
	mode        string
	publishable string
	secret      string
	webhook     string
}

// paypalEnv 是启动时解析的 PayPal 环境变量覆盖。
type paypalEnv struct {
	active   bool
	mode     string
	clientID string
	secret   string
}

func resolveStripeEnv(getenv func(string) string) stripeEnv {
	secret := strings.TrimSpace(getenv(envStripeSecret))
	if secret == "" {
		return stripeEnv{}
	}
	mode := strings.TrimSpace(getenv(envStripeMode))
	if mode == "" {
		if strings.Contains(secret, "_live_") {
			mode = "live"
		} else {
			mode = "test"
		}
	}
	return stripeEnv{
		active: true, mode: mode,
		publishable: strings.TrimSpace(getenv(envStripePublishable)),
		secret:      secret,
		webhook:     strings.TrimSpace(getenv(envStripeWebhook)),
	}
}

func resolvePayPalEnv(getenv func(string) string) paypalEnv {
	id := strings.TrimSpace(getenv(envPayPalClientID))
	if id == "" {
		return paypalEnv{}
	}
	mode := strings.TrimSpace(getenv(envPayPalMode))
	if mode == "" {
		mode = "sandbox"
	}
	return paypalEnv{
		active: true, mode: mode, clientID: id,
		secret: strings.TrimSpace(getenv(envPayPalSecret)),
	}
}

// KeyCache 进程级持有【已解密】的收款密钥（Stripe + PayPal）。
// 默认来源=加密库：生命周期严格绑定 KEK 金库（Unlock=登录解锁；Lock=登出销毁；改密钥后立即重载）。
// 每通道可选 env 覆盖：若该通道 env 激活，启动即在、优先采用、不读/不解密该通道库内值（覆盖非双写）。
type KeyCache struct {
	settings  *settings.Service
	stripeEnv stripeEnv
	paypalEnv paypalEnv

	mu sync.RWMutex
	// Stripe（库来源）
	sMode, sPublishable, sSecret, sWebhook string
	// PayPal（库来源）
	pMode, pClientID, pSecret string
}

// NewKeyCache 构造缓存。读取一次环境变量覆盖（不落库、不进日志）。
func NewKeyCache(s *settings.Service) *KeyCache {
	return &KeyCache{
		settings:  s,
		stripeEnv: resolveStripeEnv(os.Getenv),
		paypalEnv: resolvePayPalEnv(os.Getenv),
	}
}

// EnvOverride 报告是否有任一通道处于环境变量覆盖模式。
func (c *KeyCache) EnvOverride() bool { return c.stripeEnv.active || c.paypalEnv.active }

// Unlock 用 KEK 解密收款密钥载入内存（库来源默认路径）。
// 某通道 env 覆盖激活时，该通道为 no-op：不读、不解密其库内值（杜绝跨源混用）。
func (c *KeyCache) Unlock(ctx context.Context, kek []byte) error {
	var sMode, sPub, sSecret, sWebhook, pMode, pID, pSecret string

	if !c.stripeEnv.active {
		sMode, _ = c.settings.Get(ctx, KeyStripeMode)
		if strings.TrimSpace(sMode) == "" {
			sMode = "test"
		}
		sPub, _ = c.settings.Get(ctx, KeyStripePublishable)
		b1, err := c.settings.GetEncrypted(ctx, KeyStripeSecret, kek)
		if err != nil && !errors.Is(err, settings.ErrNotFound) {
			return err
		}
		b2, err := c.settings.GetEncrypted(ctx, KeyStripeWebhookSecret, kek)
		if err != nil && !errors.Is(err, settings.ErrNotFound) {
			return err
		}
		sSecret, sWebhook = string(b1), string(b2)
	}

	if !c.paypalEnv.active {
		pMode, _ = c.settings.Get(ctx, KeyPayPalMode)
		if strings.TrimSpace(pMode) == "" {
			pMode = "sandbox"
		}
		pID, _ = c.settings.Get(ctx, KeyPayPalClientID)
		b3, err := c.settings.GetEncrypted(ctx, KeyPayPalSecret, kek)
		if err != nil && !errors.Is(err, settings.ErrNotFound) {
			return err
		}
		pSecret = string(b3)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.sMode, c.sPublishable, c.sSecret, c.sWebhook = sMode, sPub, sSecret, sWebhook
	c.pMode, c.pClientID, c.pSecret = pMode, pID, pSecret
	return nil
}

// Lock 销毁库来源的内存密钥（登出时调用）。不影响 env 覆盖（其值随进程常驻）。
func (c *KeyCache) Lock() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sMode, c.sPublishable, c.sSecret, c.sWebhook = "", "", "", ""
	c.pMode, c.pClientID, c.pSecret = "", "", ""
}

// secretKey 返回 Stripe sk_*；未解锁/未配置返回 false。env 覆盖优先。
func (c *KeyCache) secretKey() (string, bool) {
	if c.stripeEnv.active {
		return c.stripeEnv.secret, c.stripeEnv.secret != ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sSecret, c.sSecret != ""
}

// webhookSecret 返回 Stripe whsec_*；未解锁/未配置返回 false。env 覆盖优先。
func (c *KeyCache) webhookSecret() (string, bool) {
	if c.stripeEnv.active {
		return c.stripeEnv.webhook, c.stripeEnv.webhook != ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sWebhook, c.sWebhook != ""
}

// paypalCreds 返回 PayPal (clientID, secret, mode)；缺任一返回 ok=false。env 覆盖优先。
func (c *KeyCache) paypalCreds() (clientID, secret, mode string, ok bool) {
	if c.paypalEnv.active {
		e := c.paypalEnv
		return e.clientID, e.secret, e.mode, e.clientID != "" && e.secret != ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pClientID, c.pSecret, c.pMode, c.pClientID != "" && c.pSecret != ""
}

// Status 返回不含密钥明文的状态快照（双通道）。
func (c *KeyCache) Status() CacheStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st := CacheStatus{StripeSource: "db", PayPalSource: "db"}

	if c.stripeEnv.active {
		e := c.stripeEnv
		st.StripeSource, st.StripeMode, st.StripePublishable = "env", e.mode, e.publishable
		st.StripeHasSecret, st.StripeHasWebhook = e.secret != "", e.webhook != ""
	} else {
		st.StripeMode, st.StripePublishable = orDefault(c.sMode, "test"), c.sPublishable
		st.StripeHasSecret, st.StripeHasWebhook = c.sSecret != "", c.sWebhook != ""
	}

	if c.paypalEnv.active {
		e := c.paypalEnv
		st.PayPalSource, st.PayPalMode, st.PayPalClientID = "env", e.mode, e.clientID
		st.PayPalHasSecret = e.secret != ""
	} else {
		st.PayPalMode, st.PayPalClientID = orDefault(c.pMode, "sandbox"), c.pClientID
		st.PayPalHasSecret = c.pSecret != ""
	}
	return st
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
