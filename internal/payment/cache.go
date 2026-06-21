// 收款密钥内存缓存 / In-Memory Payment Key Cache
// 功能：默认从加密库(KEK)解密载入收款密钥(登录解锁/登出销毁/改密钥重载)；可选环境变量覆盖旁路(env>加密库,覆盖非双写)
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-21 10:30:00
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
)

// CacheStatus 是不含密钥明文的状态快照，供后台收款页/诊断展示。
type CacheStatus struct {
	Source      string // env | db：当前生效的密钥来源
	Mode        string // test | live
	Publishable string // 可发布密钥（公开值，可明文展示）
	HasSecret   bool   // 是否已配置 sk
	HasWebhook  bool   // 是否已配置 whsec
}

// envKeys 是启动时解析的环境变量覆盖；active=false 表示未覆盖（走加密库默认路径）。
type envKeys struct {
	active      bool
	mode        string
	publishable string
	secret      string
	webhook     string
}

// resolveEnvKeys 解析环境变量覆盖。仅当 STRIPE_SECRET_KEY 设置时才视为「env 覆盖激活」，
// 此时全部来源取自 env（不回填库内值，杜绝跨源混用）；否则不激活、走加密库默认路径。
func resolveEnvKeys(getenv func(string) string) envKeys {
	secret := strings.TrimSpace(getenv(envStripeSecret))
	if secret == "" {
		return envKeys{}
	}
	mode := strings.TrimSpace(getenv(envStripeMode))
	if mode == "" {
		// 兼容密钥(sk_)与受限密钥(rk_，Stripe 推荐)：按 _live_ 段判定，避免 rk_live_ 被误判为 test。
		if strings.Contains(secret, "_live_") {
			mode = "live"
		} else {
			mode = "test"
		}
	}
	return envKeys{
		active:      true,
		mode:        mode,
		publishable: strings.TrimSpace(getenv(envStripePublishable)),
		secret:      secret,
		webhook:     strings.TrimSpace(getenv(envStripeWebhook)),
	}
}

// KeyCache 进程级持有【已解密】的收款密钥。
// 默认来源=加密库：生命周期严格绑定 KEK 金库（Unlock=登录解锁；Lock=登出销毁；改密钥后立即重载）。
// 可选来源=环境变量覆盖：若 env 激活，启动即在、优先采用、不读/不解密库内值；此模式下不存在「锁定」态。
// 锁定（库模式未解锁或无密钥）时依赖它的能力一律不可用——Webhook 据此返回非 2xx 交网关重投，绝不放行。
type KeyCache struct {
	settings *settings.Service
	env      envKeys // 启动时解析、不可变；active 时覆盖库内值

	mu          sync.RWMutex
	mode        string
	publishable string
	secret      string // sk_*（库来源）
	webhook     string // whsec_*（库来源）
}

// NewKeyCache 构造缓存。读取一次环境变量覆盖（不落库、不进日志）。
func NewKeyCache(s *settings.Service) *KeyCache {
	return &KeyCache{settings: s, env: resolveEnvKeys(os.Getenv)}
}

// EnvOverride 报告是否处于环境变量覆盖模式。
func (c *KeyCache) EnvOverride() bool { return c.env.active }

// Unlock 用 KEK 解密收款密钥载入内存（库来源默认路径）。
// env 覆盖激活时为 no-op：不读、不解密库内值（杜绝跨源混用）。
func (c *KeyCache) Unlock(ctx context.Context, kek []byte) error {
	if c.env.active {
		return nil
	}
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

// Lock 销毁库来源的内存密钥（登出时调用）。不影响 env 覆盖（其值随进程常驻、非密钥金库托管）。
// 注：Go 字符串不可变，无法原地清零；此处丢弃引用，等同金库失效。
func (c *KeyCache) Lock() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode, c.publishable, c.secret, c.webhook = "", "", "", ""
}

// secretKey 返回 sk_*；未解锁/未配置返回 false。env 覆盖优先。
func (c *KeyCache) secretKey() (string, bool) {
	if c.env.active {
		return c.env.secret, c.env.secret != ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.secret == "" {
		return "", false
	}
	return c.secret, true
}

// webhookSecret 返回 whsec_*；未解锁/未配置返回 false。env 覆盖优先（env 激活但未设 whsec 仍返 false→Webhook 锁定，属预期）。
func (c *KeyCache) webhookSecret() (string, bool) {
	if c.env.active {
		return c.env.webhook, c.env.webhook != ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.webhook == "" {
		return "", false
	}
	return c.webhook, true
}

// Status 返回不含密钥明文的状态快照。
func (c *KeyCache) Status() CacheStatus {
	if c.env.active {
		return CacheStatus{
			Source: "env", Mode: c.env.mode, Publishable: c.env.publishable,
			HasSecret: c.env.secret != "", HasWebhook: c.env.webhook != "",
		}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	mode := c.mode
	if mode == "" {
		mode = "test"
	}
	return CacheStatus{
		Source: "db", Mode: mode, Publishable: c.publishable,
		HasSecret: c.secret != "", HasWebhook: c.webhook != "",
	}
}
