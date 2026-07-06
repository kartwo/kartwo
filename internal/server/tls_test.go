// 自动 HTTPS 单元测试 / Automatic HTTPS Tests
// 功能：域名来源优先级(env 覆盖 DB)、HostPolicy 白名单、HSTS 门控、证书目录权限
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-06 10:49:17
package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/acme/autocert"
)

// fakeDomainDB 是 DomainReader 的测试替身。
type fakeDomainDB struct {
	domain string
	err    error
}

func (f fakeDomainDB) Domain(context.Context) (string, error) { return f.domain, f.err }

func TestEffectiveDomain(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		envDomain  string
		db         DomainReader
		wantDomain string
		wantSource string
	}{
		{"env 覆盖 DB（DB 亦有值也不采）", "shop.example.com", fakeDomainDB{domain: "other.example.com"}, "shop.example.com", "env"},
		{"env 空则读 DB", "", fakeDomainDB{domain: "db.example.com"}, "db.example.com", "db"},
		{"两者皆空 → 评估态", "", fakeDomainDB{domain: ""}, "", "none"},
		{"env 空且 DB 报错 → 评估态", "", fakeDomainDB{err: errors.New("boom")}, "", "none"},
		{"DB 为 nil → 评估态", "", nil, "", "none"},
		{"env 带空白被裁剪", "  shop.example.com  ", nil, "shop.example.com", "env"},
		{"DB 值带空白被裁剪", "", fakeDomainDB{domain: "  db.example.com  "}, "db.example.com", "db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDomain, gotSource := EffectiveDomain(ctx, tc.envDomain, tc.db)
			if gotDomain != tc.wantDomain || gotSource != tc.wantSource {
				t.Fatalf("EffectiveDomain=(%q,%q) 期望=(%q,%q)", gotDomain, gotSource, tc.wantDomain, tc.wantSource)
			}
		})
	}
}

func TestHostPolicy(t *testing.T) {
	ctx := context.Background()
	pol := hostPolicy("shop.example.com")
	if err := pol(ctx, "shop.example.com"); err != nil {
		t.Fatalf("放行生效域名应无错，得 %v", err)
	}
	if err := pol(ctx, "SHOP.EXAMPLE.COM"); err != nil {
		t.Fatalf("大小写不敏感匹配应放行，得 %v", err)
	}
	if err := pol(ctx, "evil.example.com"); err == nil {
		t.Fatal("非白名单域名必须拒绝，却放行了")
	}
	if err := pol(ctx, ""); err == nil {
		t.Fatal("空 host 必须拒绝")
	}
	// 空域名策略：一律拒绝（不应给任意 host 签发）。
	empty := hostPolicy("")
	if err := empty(ctx, "anything.example.com"); err == nil {
		t.Fatal("空域名策略必须拒绝任意 host")
	}
}

func TestSecurityHeaders_HSTSGating(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// HTTPS 启用 → 发 HSTS。
	on := securityHeaders(true)(next)
	recOn := httptest.NewRecorder()
	on.ServeHTTP(recOn, httptest.NewRequest(http.MethodGet, "/", nil))
	if recOn.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HTTPS 启用时应注入 HSTS 头")
	}

	// HTTP-only 评估态 → 严禁 HSTS，但其它安全头仍在。
	off := securityHeaders(false)(next)
	recOff := httptest.NewRecorder()
	off.ServeHTTP(recOff, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := recOff.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HTTP-only 评估态严禁发 HSTS，却得 %q", got)
	}
	if recOff.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("非 HSTS 的其它安全头应始终存在")
	}
	if recOff.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("CSP 头应始终存在")
	}
}

func TestNewCertManager(t *testing.T) {
	certDir := filepath.Join(t.TempDir(), "certs")

	// 空 ACME 目录 → LE 生产（Client 为 nil，走 autocert 默认）。
	m, err := NewCertManager("shop.example.com", certDir, "")
	if err != nil {
		t.Fatalf("NewCertManager 失败: %v", err)
	}
	if m.Client != nil {
		t.Fatal("空 ACME 目录应留 Client=nil（autocert 默认 LE 生产）")
	}
	if _, ok := m.Cache.(autocert.DirCache); !ok {
		t.Fatalf("证书缓存应为 DirCache，得 %T", m.Cache)
	}
	// 证书目录必须以 0700 建立（明文缓存，KEK 例外，需最严目录权限）。
	info, err := os.Stat(certDir)
	if err != nil {
		t.Fatalf("证书目录未建立: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("证书目录权限应为 0700，得 %o", perm)
	}

	// 指定 ACME 目录（如 LE Staging）→ Client.DirectoryURL 钉死之。
	const staging = "https://acme-staging-v02.api.letsencrypt.org/directory"
	m2, err := NewCertManager("shop.example.com", filepath.Join(t.TempDir(), "certs"), staging)
	if err != nil {
		t.Fatalf("NewCertManager(staging) 失败: %v", err)
	}
	if m2.Client == nil || m2.Client.DirectoryURL != staging {
		t.Fatalf("指定 ACME 目录应钉死 Client.DirectoryURL=%q，得 %+v", staging, m2.Client)
	}
}
