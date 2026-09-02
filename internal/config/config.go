// 配置加载 / Configuration Loader
// 功能：从环境变量加载运行配置并提供安全默认值；不读取/记录任何密钥
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config 为内核运行所需的最小配置（M0）。
// 注意：支付/SMTP/会话等密钥不在此结构中——它们后续加密存库（见 ARCHITECTURE §14/§15），绝不走环境明文。
type Config struct {
	Env      string // 运行环境：dev | prod
	Addr     string // dev HTTP 监听地址，如 :8080（prod 用 HTTPAddr/HTTPSAddr）
	DataDir  string // 数据目录，默认 ./data（数据即文件夹）
	DBEngine string // 数据库引擎：sqlite（默认）| postgres（升级项，M0 未实现）
	DBPath   string // SQLite 数据库文件路径（由 DataDir 派生）
	ShopName string // 店铺名（店面展示/SEO），默认占位，向导完整化于 M4
	Currency string // 币种代码（CNY/USD/EUR…），默认 CNY
	BaseURL  string // 站点基址（用于 canonical/sitemap 绝对 URL）；空则按请求推导
	// TrustedProxies 为可采信 X-Forwarded-Proto 的上游代理 CIDR/IP 白名单。
	TrustedProxies []*net.IPNet

	// —— M4.1 自动 HTTPS（仅 prod 生效）——
	// Domain 为 env 覆盖来源的"当前生效域名"：非空即用且视为 locked（env>DB、覆盖非双写，
	// 与支付密钥 env 覆盖纪律同形）；env 空时由 settings 的 domain 键提供（向导写入，M4.2）。
	Domain string // KARTWO_DOMAIN；空则回退读 DB
	// HTTPAddr prod 明文端口：服 ACME HTTP-01 challenge 并 302 跳 HTTPS；HTTP-only 评估态下直接服应用。
	HTTPAddr string // KARTWO_HTTP_ADDR，默认 :80
	// HTTPSAddr prod TLS 端口：域名就位时经 autocert 自动签发证书对外服 HTTPS。
	HTTPSAddr string // KARTWO_HTTPS_ADDR，默认 :443
	// ACMEDirectory 为 ACME 目录 URL；空=Let's Encrypt 生产。设为 LE Staging 可预跑不烧生产配额。
	ACMEDirectory string // KARTWO_ACME_DIRECTORY，默认空（LE 生产）

	// —— M5.8 本地自动备份 ——
	// BackupInterval 控制服务启动后及其后的本地全量 ZIP 备份频率，默认 24h。
	BackupInterval time.Duration
	// BackupRetention 是仅由程序创建的持久备份 ZIP 保留份数，默认 7。
	BackupRetention int
	// BackupIntervalEnv / BackupRetentionEnv 标记对应项是否被环境变量显式覆盖。
	BackupIntervalEnv  bool
	BackupRetentionEnv bool

	// —— M5.9 备份到 WebDAV ——
	// BackupWebDAVEnabled 控制是否启用异地 WebDAV 上传；默认 false。
	BackupWebDAVEnabled bool
	// BackupWebDAVURL 为异地 WebDAV 接入点（必须是 https://）。空即未配置。
	BackupWebDAVURL string
	// BackupWebDAVPath 为 WebDAV 中备份上传目录，默认 /.
	BackupWebDAVPath string
	// BackupWebDAVUsername 用于 Basic 认证；空则不带认证头。
	BackupWebDAVUsername string
	// BackupWebDAVPassword 优先使用环境变量；若未配置则为空。
	BackupWebDAVPassword string
	// BackupWebDAVEnabledEnv/BackupWebDAVURLEnv/BackupWebDAVPathEnv/BackupWebDAVUsernameEnv
	// / BackupWebDAVPasswordEnv 标记对应项是否被环境变量显式覆盖。
	BackupWebDAVEnabledEnv bool
	BackupWebDAVURLEnv      bool
	BackupWebDAVPathEnv     bool
	BackupWebDAVUsernameEnv bool
	BackupWebDAVPasswordEnv bool
}

const (
	// BackupIntervalSetting / BackupRetentionSetting 是后台持久化的自部署备份设置键。
	BackupIntervalSetting       = "backup.interval"
	BackupRetentionSetting      = "backup.retention"
	BackupWebDAVEnabledSetting  = "backup.webdav.enabled" // #nosec G101 -- 持久化设置键名，不含凭证值。
	BackupWebDAVURLSetting      = "backup.webdav.url" // #nosec G101 -- 持久化设置键名，不含凭证值。
	BackupWebDAVPathSetting     = "backup.webdav.path" // #nosec G101 -- 持久化设置键名，不含凭证值。
	BackupWebDAVUsernameSetting = "backup.webdav.username" // #nosec G101 -- 持久化设置键名，不含凭证值。
	BackupWebDAVPasswordSetting = "backup.webdav.password" // #nosec G101 -- 持久化设置键名，不含凭证值。
)

// Load 从环境变量读取配置并填默认值。
// 双模式纪律：此处只做自部署默认语义，不感知 SaaS。
func Load() (*Config, error) {
	cfg := &Config{
		Env:             getEnv("KARTWO_ENV", "dev"),
		Addr:            getEnv("KARTWO_ADDR", ":8080"),
		DataDir:         getEnv("KARTWO_DATA_DIR", "./data"),
		DBEngine:        getEnv("KARTWO_DB_ENGINE", "sqlite"),
		ShopName:        getEnv("KARTWO_SHOP_NAME", "Kartwo Store"),
		Currency:        getEnv("KARTWO_CURRENCY", "CNY"),
		BaseURL:         getEnv("KARTWO_BASE_URL", ""),
		Domain:          strings.TrimSpace(getEnv("KARTWO_DOMAIN", "")),
		HTTPAddr:        getEnv("KARTWO_HTTP_ADDR", ":80"),
		HTTPSAddr:       getEnv("KARTWO_HTTPS_ADDR", ":443"),
		ACMEDirectory:   strings.TrimSpace(getEnv("KARTWO_ACME_DIRECTORY", "")),
		BackupInterval:   24 * time.Hour,
		BackupRetention:  7,
		BackupWebDAVPath: "/",
	}
	if raw, ok := os.LookupEnv("KARTWO_TRUSTED_PROXIES"); ok {
		parsed, err := ParseTrustedProxies(raw)
		if err != nil {
			return nil, fmt.Errorf("非法 KARTWO_TRUSTED_PROXIES=%q（%v）", raw, err)
		}
		cfg.TrustedProxies = parsed
	}

	switch cfg.Env {
	case "dev", "prod":
	default:
		return nil, fmt.Errorf("非法 KARTWO_ENV=%q（应为 dev 或 prod）", cfg.Env)
	}

	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("KARTWO_ADDR 不能为空")
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_INTERVAL"); ok && raw != "" {
		interval, err := ParseBackupInterval(raw)
		if err != nil {
			return nil, fmt.Errorf("非法 KARTWO_BACKUP_INTERVAL=%q（应为不小于 1m 的 Go duration）", raw)
		}
		cfg.BackupInterval, cfg.BackupIntervalEnv = interval, true
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_RETENTION"); ok && raw != "" {
		retention, err := ParseBackupRetention(raw)
		if err != nil {
			return nil, fmt.Errorf("非法 KARTWO_BACKUP_RETENTION=%q（应为 1 到 365）", raw)
		}
		cfg.BackupRetention, cfg.BackupRetentionEnv = retention, true
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_WEBDAV_ENABLED"); ok && raw != "" {
		enabled, err := ParseBackupWebDAVEnabled(raw)
		if err != nil {
			return nil, fmt.Errorf("非法 KARTWO_BACKUP_WEBDAV_ENABLED=%q（应为 true / false）", raw)
		}
		cfg.BackupWebDAVEnabled, cfg.BackupWebDAVEnabledEnv = enabled, true
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_WEBDAV_URL"); ok && raw != "" {
		parsed, err := ParseBackupWebDAVURL(raw)
		if err != nil {
			return nil, fmt.Errorf("非法 KARTWO_BACKUP_WEBDAV_URL=%q（%v）", raw, err)
		}
		cfg.BackupWebDAVURL = parsed
		cfg.BackupWebDAVURLEnv = true
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_WEBDAV_PATH"); ok && raw != "" {
		if parsed, err := ParseBackupWebDAVPath(raw); err != nil {
			return nil, fmt.Errorf("非法 KARTWO_BACKUP_WEBDAV_PATH=%q（%v）", raw, err)
		} else {
			cfg.BackupWebDAVPath = parsed
			cfg.BackupWebDAVPathEnv = true
		}
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_WEBDAV_USERNAME"); ok {
		cfg.BackupWebDAVUsername = strings.TrimSpace(raw)
		cfg.BackupWebDAVUsernameEnv = true
	}
	if raw, ok := os.LookupEnv("KARTWO_BACKUP_WEBDAV_PASSWORD"); ok {
		cfg.BackupWebDAVPassword = raw
		cfg.BackupWebDAVPasswordEnv = true
	}

	// M0 仅落地 sqlite 默认实现；postgres 作为升级项接口占位。
	if cfg.DBEngine != "sqlite" {
		return nil, fmt.Errorf("KARTWO_DB_ENGINE=%q 暂未实现（M0 仅支持 sqlite）", cfg.DBEngine)
	}

	cfg.DBPath = filepath.Join(cfg.DataDir, "shop.db")
	return cfg, nil
}

// ParseBackupInterval 校验后台与环境变量共同使用的备份周期格式。
func ParseBackupInterval(raw string) (time.Duration, error) {
	interval, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || interval < time.Minute {
		return 0, fmt.Errorf("备份周期应为不小于 1m 的 Go duration")
	}
	return interval, nil
}

// ParseTrustedProxies 解析可信代理白名单。
// 支持 CIDR（如 203.0.113.0/24）和单 IP（如 198.51.100.10）。
func ParseTrustedProxies(raw string) ([]*net.IPNet, error) {
	parts := strings.Split(raw, ",")
	out := make([]*net.IPNet, 0, len(parts))
	for _, item := range parts {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if strings.Contains(v, "/") {
			_, c, err := net.ParseCIDR(v)
			if err != nil {
				return nil, fmt.Errorf("代理白名单项 %q 不是合法 CIDR", v)
			}
			out = append(out, c)
			continue
		}

		ip := net.ParseIP(v)
		if ip == nil {
			return nil, fmt.Errorf("代理白名单项 %q 不是合法 IP", v)
		}
		bits := 128
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
			bits = 32
		}
		out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return out, nil
}

// ParseBackupWebDAVEnabled 校验布尔表达式。
func ParseBackupWebDAVEnabled(raw string) (bool, error) {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("WebDAV 开关应为 true / false")
	}
	return value, nil
}

// ParseBackupWebDAVURL 校验 WebDAV URL（必须是 https URL）。
func ParseBackupWebDAVURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("WebDAV 地址需为合法 URL")
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("WebDAV 地址仅支持 https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("WebDAV 地址不能包含用户名或密码")
	}
	return parsed.String(), nil
}

// ParseBackupWebDAVPath 校验远端目录路径。空则转成根目录。
func ParseBackupWebDAVPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "/", nil
	}
	if strings.Contains(path, " ") {
		return "", fmt.Errorf("WebDAV 路径不能包含空白字符")
	}
	if path[0] != '/' {
		return "", fmt.Errorf("WebDAV 路径必须以 / 开头")
	}
	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path, nil
}

// ParseBackupRetention 校验后台与环境变量共同使用的保留份数。
func ParseBackupRetention(raw string) (int, error) {
	retention, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || retention < 1 || retention > 365 {
		return 0, fmt.Errorf("保留份数应为 1 到 365")
	}
	return retention, nil
}

// ApplyBackupSettings 在未被环境变量覆盖时采用数据库设置；不存在时保留默认值。
func (c *Config) ApplyBackupSettings(get func(string) (string, error)) error {
	if !c.BackupIntervalEnv {
		if raw, err := get(BackupIntervalSetting); err == nil && strings.TrimSpace(raw) != "" {
			interval, err := ParseBackupInterval(raw)
			if err != nil {
				return fmt.Errorf("数据库备份周期无效: %w", err)
			}
			c.BackupInterval = interval
		}
	}
	if !c.BackupRetentionEnv {
		if raw, err := get(BackupRetentionSetting); err == nil && strings.TrimSpace(raw) != "" {
			retention, err := ParseBackupRetention(raw)
			if err != nil {
				return fmt.Errorf("数据库备份保留数无效: %w", err)
			}
			c.BackupRetention = retention
		}
	}
	if !c.BackupWebDAVEnabledEnv {
		if raw, err := get(BackupWebDAVEnabledSetting); err == nil && strings.TrimSpace(raw) != "" {
			enabled, err := ParseBackupWebDAVEnabled(raw)
			if err != nil {
				return fmt.Errorf("数据库备份 WebDAV 开关无效: %w", err)
			}
			c.BackupWebDAVEnabled = enabled
		}
	}
	if !c.BackupWebDAVURLEnv {
		if raw, err := get(BackupWebDAVURLSetting); err == nil && strings.TrimSpace(raw) != "" {
			u, err := ParseBackupWebDAVURL(raw)
			if err != nil {
				return fmt.Errorf("数据库备份 WebDAV 地址无效: %w", err)
			}
			c.BackupWebDAVURL = u
		}
	}
	if !c.BackupWebDAVPathEnv {
		if raw, err := get(BackupWebDAVPathSetting); err == nil && strings.TrimSpace(raw) != "" {
			p, err := ParseBackupWebDAVPath(raw)
			if err != nil {
				return fmt.Errorf("数据库备份 WebDAV 路径无效: %w", err)
			}
			c.BackupWebDAVPath = p
		}
	}
	if !c.BackupWebDAVUsernameEnv {
		if raw, err := get(BackupWebDAVUsernameSetting); err == nil {
			c.BackupWebDAVUsername = strings.TrimSpace(raw)
		}
	}
	return nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
