// 自动 HTTPS 装配 / Automatic HTTPS (autocert)
// 功能：域名来源解析(env 覆盖 DB)、autocert 证书管理器、HostPolicy 白名单、HTTP→HTTPS 跳转、DirCache 明文证书缓存
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-06 10:49:17
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// DomainReader 提供 DB 中配置的域名（由 settings.Service 实现）。
// server 包只依赖此最小接口，避免与 settings 耦合，也便于单测注入。
type DomainReader interface {
	Domain(ctx context.Context) (string, error)
}

// EffectiveDomain 解析"当前生效域名"及其来源，遵循 env 覆盖 DB 的凭证来源纪律：
//   - env（envDomain 非空）→ 用之，来源 "env"（不再读 DB，不双写、不回退）；
//   - env 空 → 读 DB；DB 非空 → 来源 "db"；
//   - 两者皆空 → 返回 ""，来源 "none"（HTTP-only 评估态）。
func EffectiveDomain(ctx context.Context, envDomain string, db DomainReader) (domain, source string) {
	if d := strings.TrimSpace(envDomain); d != "" {
		return d, "env"
	}
	if db != nil {
		if d, err := db.Domain(ctx); err == nil {
			if d = strings.TrimSpace(d); d != "" {
				return d, "db"
			}
		}
	}
	return "", "none"
}

// hostPolicy 返回只放行单一生效域名的 autocert HostPolicy（白名单）。
// 拒绝其他 host，防他人把域名解析到本机骗取证书、烧 Let's Encrypt 速率配额。
func hostPolicy(domain string) autocert.HostPolicy {
	allowed := strings.ToLower(strings.TrimSpace(domain))
	return func(_ context.Context, host string) error {
		if allowed != "" && strings.ToLower(strings.TrimSpace(host)) == allowed {
			return nil
		}
		return fmt.Errorf("server: 拒绝为非白名单域名 %q 签发证书（仅允许 %q）", host, allowed)
	}
}

// NewCertManager 装配 autocert 证书管理器。
//   - certDir：证书缓存目录（DirCache，明文 0600/目录 0700），启动无需主口令即可读——
//     TLS 必须先于任何登录解锁 KEK 起服，故此为"凭证一律 KEK 加密"的显式例外（见 DECISIONS）。
//   - acmeDirectory：ACME 目录 URL，空=Let's Encrypt 生产；设为 LE Staging 可预跑不烧生产配额。
//
// 返回 error 仅在证书目录无法以 0700 建立时（尽早暴露权限问题）。
func NewCertManager(domain, certDir, acmeDirectory string) (*autocert.Manager, error) {
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return nil, fmt.Errorf("server: 建证书缓存目录失败 %q: %w", certDir, err)
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(certDir),
		HostPolicy: hostPolicy(domain),
	}
	if acmeDirectory != "" {
		m.Client = &acme.Client{DirectoryURL: acmeDirectory}
	}
	return m, nil
}

// TLSConfig 基于 autocert 管理器构造对外 HTTPS 的 tls.Config，钉最低 TLS1.2。
func TLSConfig(m *autocert.Manager) *tls.Config {
	cfg := m.TLSConfig()
	cfg.MinVersion = tls.VersionTLS12
	return cfg
}

// httpsRedirect 把明文请求 301 跳到同域 HTTPS（保留路径与查询串）。
// autocert 的 HTTPHandler 会先截获 /.well-known/acme-challenge/*，其余交此 fallback。
func httpsRedirect(domain string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + domain + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// ChallengeHandler 返回 prod HTTP(:80) 端的处理器：ACME challenge + 其余 302→HTTPS。
func ChallengeHandler(m *autocert.Manager, domain string) http.Handler {
	return m.HTTPHandler(httpsRedirect(domain))
}
