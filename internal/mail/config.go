// SMTP 配置与内存缓存 / SMTP Config & In-Memory Cache
// 功能：SMTP 设置键定义；env 覆盖旁路(D1)；KEK 解密载入密码入内存(镜像 payment 缓存：登录解锁/登出销毁)；配置快照供 worker/测试发信/设置页
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package mail

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/kartwo/kartwo/internal/settings"
)

// SMTP 设置键（明文项走 SetPlain，password 走 SetEncrypted(KEK)）。
const (
	KeySMTPHost        = "smtp.host"
	KeySMTPPort        = "smtp.port"
	KeySMTPUsername    = "smtp.username"
	KeySMTPPassword    = "smtp.password" //nolint:gosec // 设置「键名」，非凭证本身
	KeySMTPFromAddress = "smtp.from_address"
	KeySMTPFromName    = "smtp.from_name"
	KeySMTPEncryption  = "smtp.encryption" // none | starttls | tls
)

// 环境变量名（D1：可选 env 覆盖旁路，让 worker 不依赖登录、冷启动即可发信；env>加密库、覆盖非双写）。
const (
	envSMTPHost       = "SMTP_HOST"
	envSMTPPort       = "SMTP_PORT"
	envSMTPUsername   = "SMTP_USERNAME"
	envSMTPPassword   = "SMTP_PASSWORD" //nolint:gosec // 环境变量「名」，非凭证本身
	envSMTPFrom       = "SMTP_FROM_ADDRESS"
	envSMTPFromName   = "SMTP_FROM_NAME"
	envSMTPEncryption = "SMTP_ENCRYPTION"
)

// Config 是一份完整 SMTP 配置（含密码明文，仅在内存/发送时用，绝不落日志）。
type Config struct {
	Host        string
	Port        string
	Username    string
	Password    string
	FromAddress string
	FromName    string
	Encryption  string // none | starttls | tls
}

// Configured 报告是否可发信（主机/端口/发件地址齐备；用户名密码可空，兼容无鉴权中继如 MailHog）。
func (c Config) Configured() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.Port) != "" && strings.TrimSpace(c.FromAddress) != ""
}

// Status 是不含密码明文的状态快照，供后台 SMTP 设置页/诊断展示。
type Status struct {
	Source      string // env | db
	Host        string
	Port        string
	Username    string
	FromAddress string
	FromName    string
	Encryption  string
	HasPassword bool
	Configured  bool
}

type smtpEnv struct {
	active bool
	cfg    Config
}

func resolveSMTPEnv(getenv func(string) string) smtpEnv {
	host := strings.TrimSpace(getenv(envSMTPHost))
	if host == "" {
		return smtpEnv{}
	}
	enc := strings.TrimSpace(getenv(envSMTPEncryption))
	if enc == "" {
		enc = "starttls"
	}
	return smtpEnv{active: true, cfg: Config{
		Host:        host,
		Port:        orDefault(strings.TrimSpace(getenv(envSMTPPort)), "587"),
		Username:    strings.TrimSpace(getenv(envSMTPUsername)),
		Password:    getenv(envSMTPPassword), // 不 trim：密码可能含前后空白
		FromAddress: strings.TrimSpace(getenv(envSMTPFrom)),
		FromName:    strings.TrimSpace(getenv(envSMTPFromName)),
		Encryption:  enc,
	}}
}

// Cache 进程级持有【已解密】的 SMTP 配置。
// 默认来源=加密库：生命周期绑定 KEK 金库（Unlock=登录解锁；Lock=登出销毁）。
// env 覆盖激活时：启动即在、优先采用、不读库、常驻（worker 不依赖登录）。
type Cache struct {
	settings *settings.Service
	env      smtpEnv

	mu       sync.RWMutex
	db       Config
	dbLoaded bool
}

// NewCache 构造缓存，读取一次 env 覆盖（不落库、不进日志）。
func NewCache(s *settings.Service) *Cache {
	return &Cache{settings: s, env: resolveSMTPEnv(os.Getenv)}
}

// EnvOverride 报告是否处于 env 覆盖模式。
func (c *Cache) EnvOverride() bool { return c.env.active }

// Unlock 用 KEK 解密载入 SMTP 密码（库来源）。env 覆盖时为 no-op（不读库）。
func (c *Cache) Unlock(ctx context.Context, kek []byte) error {
	if c.env.active {
		return nil
	}
	cfg := Config{}
	cfg.Host, _ = c.settings.Get(ctx, KeySMTPHost)
	cfg.Port, _ = c.settings.Get(ctx, KeySMTPPort)
	cfg.Username, _ = c.settings.Get(ctx, KeySMTPUsername)
	cfg.FromAddress, _ = c.settings.Get(ctx, KeySMTPFromAddress)
	cfg.FromName, _ = c.settings.Get(ctx, KeySMTPFromName)
	cfg.Encryption, _ = c.settings.Get(ctx, KeySMTPEncryption)
	pw, err := c.settings.GetEncrypted(ctx, KeySMTPPassword, kek)
	if err != nil && !errors.Is(err, settings.ErrNotFound) {
		return err
	}
	cfg.Password = string(pw)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.db = cfg
	c.dbLoaded = true
	return nil
}

// Lock 销毁库来源的内存配置（登出时调用）。不影响 env 覆盖。
func (c *Cache) Lock() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db = Config{}
	c.dbLoaded = false
}

// Configured 报告 SMTP 是否已"配置存在"（env 覆盖，或库里已存 host+from）——不需 KEK/未解锁也能判。
// 用于区分 worker 该 skip（真未配置）还是留 pending（已配但金库锁定，待登录补发）。
func (c *Cache) Configured(ctx context.Context) bool {
	return c.Status(ctx).Configured
}

// Config 返回当前可用 SMTP 配置及是否可发信。env 覆盖优先；库来源需已解锁。
func (c *Cache) Config() (Config, bool) {
	if c.env.active {
		return c.env.cfg, c.env.cfg.Configured()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.dbLoaded {
		return Config{}, false
	}
	return c.db, c.db.Configured()
}

// Status 返回不含密码明文的状态快照。env 覆盖时读 env；否则读库内明文项 + 密文项存在性。
func (c *Cache) Status(ctx context.Context) Status {
	if c.env.active {
		e := c.env.cfg
		return Status{
			Source: "env", Host: e.Host, Port: e.Port, Username: e.Username,
			FromAddress: e.FromAddress, FromName: e.FromName, Encryption: e.Encryption,
			HasPassword: e.Password != "", Configured: e.Configured(),
		}
	}
	get := func(k string) string { v, _ := c.settings.Get(ctx, k); return v }
	st := Status{
		Source: "db", Host: get(KeySMTPHost), Port: get(KeySMTPPort), Username: get(KeySMTPUsername),
		FromAddress: get(KeySMTPFromAddress), FromName: get(KeySMTPFromName),
		Encryption: orDefault(get(KeySMTPEncryption), "starttls"),
	}
	_, err := c.settings.Get(ctx, KeySMTPPassword)
	st.HasPassword = err == nil
	st.Configured = st.Host != "" && st.Port != "" && st.FromAddress != ""
	return st
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
