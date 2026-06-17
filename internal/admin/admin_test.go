// Admin 鉴权测试 / Admin Auth Tests
// 功能：初始化幂等、登录校验、会话鉴权、登出、CSRF、登录限流（核心安全逻辑必须单测）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"

	"database/sql"

	_ "modernc.org/sqlite"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return New(db)
}

func TestInitializeIdempotentGuard(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	if init, _ := svc.IsInitialized(ctx); init {
		t.Fatal("初始应未初始化")
	}
	if err := svc.Initialize(ctx, "admin", "supersecret"); err != nil {
		t.Fatalf("首次初始化失败: %v", err)
	}
	if init, _ := svc.IsInitialized(ctx); !init {
		t.Fatal("初始化后应为已初始化")
	}
	if err := svc.Initialize(ctx, "admin2", "supersecret2"); err != ErrAlreadyInitialized {
		t.Fatalf("重复初始化应被拒，得到: %v", err)
	}
}

func TestLoginAuthenticateLogout(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.Initialize(ctx, "admin", "supersecret"); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	if _, err := svc.Login(ctx, "admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("错误口令应失败，得到: %v", err)
	}

	sess, err := svc.Login(ctx, "admin", "supersecret")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	ac, err := svc.Authenticate(ctx, sess.Token)
	if err != nil {
		t.Fatalf("鉴权失败: %v", err)
	}
	if ac.Username != "admin" {
		t.Fatalf("身份用户名 = %q", ac.Username)
	}
	// KEK 应已派生入内存金库。
	if _, ok := svc.Key(sess.Token); !ok {
		t.Fatal("登录后应能取到该会话 KEK")
	}

	if err := svc.Logout(ctx, sess.Token); err != nil {
		t.Fatalf("登出失败: %v", err)
	}
	if _, err := svc.Authenticate(ctx, sess.Token); err != ErrUnauthorized {
		t.Fatalf("登出后鉴权应失败，得到: %v", err)
	}
	if _, ok := svc.Key(sess.Token); ok {
		t.Fatal("登出后不应再持有 KEK")
	}
}

func TestKEKStableAcrossLogins(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	_ = svc.Initialize(ctx, "admin", "supersecret")

	s1, _ := svc.Login(ctx, "admin", "supersecret")
	s2, _ := svc.Login(ctx, "admin", "supersecret")
	k1, ok1 := svc.Key(s1.Token)
	k2, ok2 := svc.Key(s2.Token)
	if !ok1 || !ok2 || !bytes.Equal(k1, k2) {
		t.Fatal("同口令两次登录应派生相同 KEK")
	}
}

// ---- HTTP 层 ----

func newHTTP(t *testing.T) (*HTTP, http.Handler) {
	svc := newSvc(t)
	h := NewHTTP(svc, false)
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

type apiResp struct {
	StatusCode int
	Cookies    []*http.Cookie
	Body       []byte
}

func doJSON(t *testing.T, mux http.Handler, method, path, body string, cookies []*http.Cookie, csrf string) apiResp {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(csrfHeader, csrf)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return apiResp{StatusCode: res.StatusCode, Cookies: res.Cookies(), Body: b}
}

func TestHTTPSetupLoginMeLogout(t *testing.T) {
	_, mux := newHTTP(t)

	// status 未初始化
	resp := doJSON(t, mux, "GET", "/admin/api/status", "", nil, "")
	var st struct{ Initialized bool }
	_ = json.Unmarshal(resp.Body, &st)
	if st.Initialized {
		t.Fatal("应未初始化")
	}

	// setup
	if resp := doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, ""); resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup 状态 = %d，期望 201", resp.StatusCode)
	}
	// 重复 setup → 409
	if resp := doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"a","password":"supersecret"}`, nil, ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("重复 setup 状态 = %d，期望 409", resp.StatusCode)
	}

	// 弱口令 setup 已被前一步占用，这里测登录
	if resp := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"wrong"}`, nil, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误口令登录状态 = %d，期望 401", resp.StatusCode)
	}

	resp = doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("登录状态 = %d，期望 200", resp.StatusCode)
	}
	cookies := resp.Cookies
	var sessionC, csrfC *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case sessionCookie:
			sessionC = c
		case csrfCookie:
			csrfC = c
		}
	}
	if sessionC == nil || csrfC == nil {
		t.Fatal("登录应下发 session 与 csrf cookie")
	}
	if !sessionC.HttpOnly {
		t.Fatal("session cookie 应为 HttpOnly")
	}
	if csrfC.HttpOnly {
		t.Fatal("csrf cookie 应可被 JS 读取（非 HttpOnly）")
	}

	// me 无 cookie → 401
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", nil, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 cookie 访问 me 状态 = %d，期望 401", resp.StatusCode)
	}
	// me 带 cookie → 200
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", []*http.Cookie{sessionC}, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("带 cookie 访问 me 状态 = %d，期望 200", resp.StatusCode)
	}

	// logout 无 CSRF → 403
	if resp := doJSON(t, mux, "POST", "/admin/api/logout", "", []*http.Cookie{sessionC}, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("无 CSRF 登出状态 = %d，期望 403", resp.StatusCode)
	}
	// logout 带 CSRF → 200
	if resp := doJSON(t, mux, "POST", "/admin/api/logout", "", []*http.Cookie{sessionC}, csrfC.Value); resp.StatusCode != http.StatusOK {
		t.Fatalf("带 CSRF 登出状态 = %d，期望 200", resp.StatusCode)
	}
	// 登出后 me → 401
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", []*http.Cookie{sessionC}, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("登出后 me 状态 = %d，期望 401", resp.StatusCode)
	}
}

func TestHTTPSetupWeakPasswordRejected(t *testing.T) {
	_, mux := newHTTP(t)
	if resp := doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"short"}`, nil, ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("弱口令 setup 状态 = %d，期望 400", resp.StatusCode)
	}
}

func TestHTTPLoginRateLimited(t *testing.T) {
	_, mux := newHTTP(t)
	_ = doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")

	// 5 次错误后第 6 次应被限流（429）。
	var last int
	for i := 0; i < 6; i++ {
		resp := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"wrong"}`, nil, "")
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("第 6 次登录状态 = %d，期望 429", last)
	}
}
