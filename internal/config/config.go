// 配置加载 / Configuration Loader
// 功能：从环境变量加载运行配置并提供安全默认值；不读取/记录任何密钥
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 为内核运行所需的最小配置（M0）。
// 注意：支付/SMTP/会话等密钥不在此结构中——它们后续加密存库（见 ARCHITECTURE §14/§15），绝不走环境明文。
type Config struct {
	Env      string // 运行环境：dev | prod
	Addr     string // HTTP 监听地址，如 :8080
	DataDir  string // 数据目录，默认 ./data（数据即文件夹）
	DBEngine string // 数据库引擎：sqlite（默认）| postgres（升级项，M0 未实现）
	DBPath   string // SQLite 数据库文件路径（由 DataDir 派生）
	ShopName string // 店铺名（店面展示/SEO），默认占位，向导完整化于 M4
	Currency string // 币种代码（CNY/USD/EUR…），默认 CNY
	BaseURL  string // 站点基址（用于 canonical/sitemap 绝对 URL）；空则按请求推导
}

// Load 从环境变量读取配置并填默认值。
// 双模式纪律：此处只做自部署默认语义，不感知 SaaS。
func Load() (*Config, error) {
	cfg := &Config{
		Env:      getEnv("KARTWO_ENV", "dev"),
		Addr:     getEnv("KARTWO_ADDR", ":8080"),
		DataDir:  getEnv("KARTWO_DATA_DIR", "./data"),
		DBEngine: getEnv("KARTWO_DB_ENGINE", "sqlite"),
		ShopName: getEnv("KARTWO_SHOP_NAME", "Kartwo Store"),
		Currency: getEnv("KARTWO_CURRENCY", "CNY"),
		BaseURL:  getEnv("KARTWO_BASE_URL", ""),
	}

	switch cfg.Env {
	case "dev", "prod":
	default:
		return nil, fmt.Errorf("非法 KARTWO_ENV=%q（应为 dev 或 prod）", cfg.Env)
	}

	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("KARTWO_ADDR 不能为空")
	}

	// M0 仅落地 sqlite 默认实现；postgres 作为升级项接口占位。
	if cfg.DBEngine != "sqlite" {
		return nil, fmt.Errorf("KARTWO_DB_ENGINE=%q 暂未实现（M0 仅支持 sqlite）", cfg.DBEngine)
	}

	cfg.DBPath = filepath.Join(cfg.DataDir, "shop.db")
	return cfg, nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
