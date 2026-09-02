// 自动 HTTPS 单元测试 / Automatic HTTPS Tests
// 功能：域名来源优先级(env 覆盖 DB)、HostPolicy 白名单、HSTS 门控、证书目录权限、TLS 噪声分级
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-06 10:49:17
package server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/acme/autocert"
)

func TestTLSHandshakeErrorLogger(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	errorLog := TLSHandshakeErrorLogger(logger)

	errorLog.Print("http: TLS handshake error from 203.0.113.9:443: client sent an HTTP request to an HTTPS server")
	if got := out.String(); !strings.Contains(got, "level=DEBUG") || !strings.Contains(got, "TLS 握手未完成") {
		t.Fatalf("握手噪声应降为 Debug，得 %q", got)
	}

	out.Reset()
	errorLog.Print("http: unexpected Serve error")
	if got := out.String(); !strings.Contains(got, "level=ERROR") || !strings.Contains(got, "HTTPS 服务错误") {
		t.Fatalf("非握手错误必须保留 Error，得 %q", got)
	}
}

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

// TestHTTPSRedirect_HostGating 锁死 R2 修复：只有 Host 匹配已配置域名才 301，
// 其它 Host（裸 IP 直连等）必须直接服务应用，作为 DNS/证书未就绪时的明文逃生路。
func TestHTTPSRedirect_HostGating(t *testing.T) {
	const domain = "m4final.kartwo.com"
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("APP"))
	})
	h := httpsRedirect(domain, app)

	cases := []struct {
		name       string
		host       string
		target     string // 请求路径（含查询串）
		wantStatus int
		wantLoc    string // 期望 Location；空表示不应跳转
	}{
		{"匹配域名 → 301 且保留路径与查询串", domain, "/admin/?a=1&b=2",
			http.StatusMovedPermanently, "https://" + domain + "/admin/?a=1&b=2"},
		{"匹配域名带端口 → 去端口后仍匹配", domain + ":80", "/", http.StatusMovedPermanently, "https://" + domain + "/"},
		{"匹配域名大小写不同 → 仍匹配", "M4Final.Kartwo.COM", "/", http.StatusMovedPermanently, "https://" + domain + "/"},
		{"匹配域名带尾点(FQDN 绝对写法) → 仍匹配", domain + ".", "/", http.StatusMovedPermanently, "https://" + domain + "/"},
		{"裸 IP 直连 → 不跳转，直接服应用（逃生路）", "203.0.113.10", "/admin/", http.StatusOK, ""},
		{"裸 IP 带端口 → 不跳转（逃生路）", "203.0.113.10:80", "/admin/", http.StatusOK, ""},
		{"IPv6 字面量带端口 → 不跳转（逃生路）", "[2001:db8::1]:80", "/", http.StatusOK, ""},
		{"其它域名解析到本机 → 不跳转（不做他人域名的跳板）", "evil.example.com", "/", http.StatusOK, ""},
		{"空 Host → 不跳转", "", "/", http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("状态码=%d 期望=%d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("Location"); got != tc.wantLoc {
				t.Fatalf("Location=%q 期望=%q", got, tc.wantLoc)
			}
			if tc.wantLoc == "" && rec.Body.String() != "APP" {
				t.Fatalf("逃生路应直接服应用，得 body=%q", rec.Body.String())
			}
		})
	}
}

// TestHTTPSRedirect_EmptyDomain 空域名（理论上不会走到此分支）绝不跳到 "https:///"。
func TestHTTPSRedirect_EmptyDomain(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("APP")) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = ""
	httpsRedirect("", app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "APP" {
		t.Fatalf("空域名应一律直接服应用，得 code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestChallengeHandler_ACMEPriority ACME challenge 路径优先于 Host 判断被 autocert 截获。
func TestChallengeHandler_ACMEPriority(t *testing.T) {
	m, err := NewCertManager("m4final.kartwo.com", filepath.Join(t.TempDir(), "certs"), "")
	if err != nil {
		t.Fatalf("NewCertManager 失败: %v", err)
	}
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("APP")) })
	h := ChallengeHandler(m, "m4final.kartwo.com", app)

	// challenge 路径：由 autocert 处理（无对应 token 时 404），绝不落到应用或跳转。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok", nil)
	req.Host = "m4final.kartwo.com"
	h.ServeHTTP(rec, req)
	if rec.Body.String() == "APP" || rec.Code == http.StatusMovedPermanently {
		t.Fatalf("ACME challenge 必须由 autocert 截获，得 code=%d body=%q", rec.Code, rec.Body.String())
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
