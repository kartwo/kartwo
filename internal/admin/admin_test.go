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
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kartwo/kartwo/internal/auth"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
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

// TestConfigSurvivesRestartAndRelogin 回归护栏：加密配置必须跨进程重启 + 重新登录持久存活。
// 信任关键路径——商家重启服务、重新登录，收款密钥等加密设置不得丢失；错误口令则绝不吐明文。
// 流程：初始化(生成并存 KEK 盐)→登录派生 KEK→加密落库收款密钥→关库(丢弃内存 KEK，模拟进程退出)
//
//	→同库重开→重新登录(读既存盐重派生 KEK)→成功解密取回原值；错误口令派生的 KEK 解密失败(ErrDecrypt)且不返明文。
func TestConfigSurvivesRestartAndRelogin(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + t.TempDir() + "/persist.db?_pragma=foreign_keys(ON)"
	const (
		username  = "merchant"
		password  = "correct-horse-battery"
		wrongPass = "correct-horse-battery!" // 仅一字之差，足以派生出不同 KEK
		secretKey = "pay.stripe.secret"
	)
	secret := []byte("sk_test_value_must_survive_restart")

	// ---- 第一次「启动」：初始化 → 登录派生 KEK → 加密落库收款密钥 ----
	db1, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	db1.SetMaxOpenConns(1)
	if _, err := migrate.Run(ctx, db1, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	svc1 := New(db1)
	if err := svc1.Initialize(ctx, username, password); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	sess1, err := svc1.Login(ctx, username, password)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	kek1, ok := svc1.Key(sess1.Token)
	if !ok {
		t.Fatal("登录后应持有会话 KEK")
	}
	if err := settings.New(db1).SetEncrypted(ctx, secretKey, secret, kek1); err != nil {
		t.Fatalf("加密落库失败: %v", err)
	}
	// 模拟进程退出：关库即丢弃内存金库里的 KEK（密文仍在磁盘）。
	if err := db1.Close(); err != nil {
		t.Fatalf("关库失败: %v", err)
	}

	// ---- 第二次「启动」：同库重开，内存 KEK 已不复存在 ----
	db2, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("重开库失败: %v", err)
	}
	db2.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db2.Close() })
	if _, err := migrate.Run(ctx, db2, migrations.FS); err != nil { // 重启迁移应幂等
		t.Fatalf("重启迁移应幂等: %v", err)
	}
	svc2 := New(db2)
	set2 := settings.New(db2)

	// 磁盘上存的应是密文，不含明文（重启后亦然）。
	if raw, err := set2.Get(ctx, secretKey); err != nil || bytes.Contains([]byte(raw), secret) {
		t.Fatalf("磁盘上不应含明文密钥: err=%v", err)
	}

	// 正确口令重新登录 → 读既存盐重派生 KEK → 解密取回原值（= 跨重启/重登持久存活）。
	sess2, err := svc2.Login(ctx, username, password)
	if err != nil {
		t.Fatalf("重启后登录失败: %v", err)
	}
	kek2, ok := svc2.Key(sess2.Token)
	if !ok {
		t.Fatal("重启后登录应重新派生 KEK")
	}
	got, err := set2.GetEncrypted(ctx, secretKey, kek2)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("重启后应能解密取回原收款密钥: err=%v got=%q", err, got)
	}

	// 错误口令派生出的 KEK 解不开（AES-GCM 认证失败），且绝不吐明文。
	wrongKEK, err := svc2.deriveKEK(ctx, wrongPass)
	if err != nil {
		t.Fatalf("派生 KEK 本身不应因口令错误而报错（错应发生在解密阶段）: %v", err)
	}
	bad, err := set2.GetEncrypted(ctx, secretKey, wrongKEK)
	if !errors.Is(err, auth.ErrDecrypt) {
		t.Fatalf("错误口令应解密失败(ErrDecrypt)，得 err=%v", err)
	}
	if bad != nil {
		t.Fatalf("解密失败时绝不应返回明文，得 %q", bad)
	}
}

// ---- HTTP 层 ----

func newHTTP(t *testing.T) (*HTTP, http.Handler) { return newHTTPEnvDomain(t, "") }

// newHTTPEnvDomain 构建 Admin HTTP，可注入 envDomain（模拟 KARTWO_DOMAIN 覆盖，测域名步骤只读态）。
func newHTTPEnvDomain(t *testing.T, envDomain string) (*HTTP, http.Handler) {
	svc := newSvc(t)
	root := t.TempDir() + "/media"
	md := media.New(svc.db, media.NewLocalBackend(root), media.NewDefaultPolicy(root, 10<<20, 0), 20)
	h := NewHTTP(svc, catalog.New(svc.db), md, settings.New(svc.db), nil, nil, envDomain, false)
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

// TestHTTPVariantPriceRequiredAndUpdate 守"价格必填、缺失/空 → 拒绝、绝不默认 0"防损失底线（创建 + 改价两路），
// 并验证改价端点：缺价 400 / 负数 400 / 0 与正数 200 / 生效 / 缺 CSRF 403。
func TestHTTPVariantPriceRequiredAndUpdate(t *testing.T) {
	_, mux := newHTTP(t)
	doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")
	lr := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
	var csrf string
	for _, c := range lr.Cookies {
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}

	// 创建：变体缺 price_cents（未提供）→ 400（防绕过前端不带价→默认 0 损失）。
	missing := `{"title":"P","slug":"pm","status":"active","options":[{"name":"尺码","values":["S"]}],"variants":[{"sku":"","quantity":0,"selections":[{"option":"尺码","value":"S"}]}]}`
	if r := doJSON(t, mux, "POST", "/admin/api/products", missing, lr.Cookies, csrf); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("创建缺价格应 400，得 %d %s", r.StatusCode, r.Body)
	}
	// 创建：显式 0 价 → 201（允许免费/赠品）。
	zero := `{"title":"P","slug":"pz","status":"active","options":[{"name":"尺码","values":["S"]}],"variants":[{"sku":"","price_cents":0,"quantity":0,"selections":[{"option":"尺码","value":"S"}]}]}`
	cr := doJSON(t, mux, "POST", "/admin/api/products", zero, lr.Cookies, csrf)
	if cr.StatusCode != http.StatusCreated {
		t.Fatalf("创建 0 价应 201，得 %d %s", cr.StatusCode, cr.Body)
	}
	var created struct {
		PublicID string `json:"public_id"`
	}
	_ = json.Unmarshal(cr.Body, &created)

	// 取变体 public_id。
	gp := doJSON(t, mux, "GET", "/admin/api/products/"+created.PublicID, "", lr.Cookies, "")
	var prod struct {
		Variants []struct {
			PublicID string `json:"public_id"`
		} `json:"variants"`
	}
	_ = json.Unmarshal(gp.Body, &prod)
	if len(prod.Variants) == 0 {
		t.Fatalf("应有变体: %s", gp.Body)
	}
	vid := prod.Variants[0].PublicID
	pricePath := "/admin/api/variants/" + vid + "/price"

	// 改价：缺 price_cents（空 body）→ 400。
	if r := doJSON(t, mux, "PATCH", pricePath, `{}`, lr.Cookies, csrf); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("改价缺价格应 400，得 %d %s", r.StatusCode, r.Body)
	}
	// 改价：负数 → 400。
	if r := doJSON(t, mux, "PATCH", pricePath, `{"price_cents":-1}`, lr.Cookies, csrf); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("改价负数应 400，得 %d %s", r.StatusCode, r.Body)
	}
	// 改价：0 → 200。
	if r := doJSON(t, mux, "PATCH", pricePath, `{"price_cents":0}`, lr.Cookies, csrf); r.StatusCode != http.StatusOK {
		t.Fatalf("改价 0 应 200，得 %d %s", r.StatusCode, r.Body)
	}
	// 改价：正数 → 200 并生效。
	if r := doJSON(t, mux, "PATCH", pricePath, `{"price_cents":8800}`, lr.Cookies, csrf); r.StatusCode != http.StatusOK {
		t.Fatalf("改价正数应 200，得 %d %s", r.StatusCode, r.Body)
	}
	gp2 := doJSON(t, mux, "GET", "/admin/api/products/"+created.PublicID, "", lr.Cookies, "")
	if !bytes.Contains(gp2.Body, []byte(`"price_cents":8800`)) {
		t.Fatalf("改价应生效为 8800: %s", gp2.Body)
	}
	// 改价缺 CSRF → 403（写操作防护）。
	if r := doJSON(t, mux, "PATCH", pricePath, `{"price_cents":100}`, lr.Cookies, ""); r.StatusCode != http.StatusForbidden {
		t.Fatalf("改价缺 CSRF 应 403，得 %d", r.StatusCode)
	}
}

func TestWizardPayment(t *testing.T) {
	_, mux := newHTTP(t)
	doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")
	lr := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
	var csrf string
	for _, c := range lr.Cookies {
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}

	// 未配收款、未跳过 → needed=true。
	r := doJSON(t, mux, "GET", "/admin/api/wizard/payment", "", lr.Cookies, "")
	if r.StatusCode != http.StatusOK || !bytes.Contains(r.Body, []byte(`"needed":true`)) {
		t.Fatalf("初始应 needed=true: %d %s", r.StatusCode, r.Body)
	}
	// 跳过（写操作需 CSRF）。
	s := doJSON(t, mux, "POST", "/admin/api/wizard/payment/skip", "", lr.Cookies, csrf)
	if s.StatusCode != http.StatusOK {
		t.Fatalf("skip 应 200: %d %s", s.StatusCode, s.Body)
	}
	// 跳过后 → needed=false。
	r2 := doJSON(t, mux, "GET", "/admin/api/wizard/payment", "", lr.Cookies, "")
	if !bytes.Contains(r2.Body, []byte(`"needed":false`)) {
		t.Fatalf("跳过后应 needed=false: %s", r2.Body)
	}
	// skip 缺 CSRF → 403。
	if bad := doJSON(t, mux, "POST", "/admin/api/wizard/payment/skip", "", lr.Cookies, ""); bad.StatusCode != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应 403，得 %d", bad.StatusCode)
	}
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

// loginAndCookies 完成 setup+login，返回 session cookie 与 csrf 值。
func loginAndCookies(t *testing.T, mux http.Handler) (*http.Cookie, string) {
	t.Helper()
	_ = doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")
	resp := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
	var sessionC *http.Cookie
	var csrf string
	for _, c := range resp.Cookies {
		switch c.Name {
		case sessionCookie:
			sessionC = c
		case csrfCookie:
			csrf = c.Value
		}
	}
	if sessionC == nil || csrf == "" {
		t.Fatal("登录未返回 session/csrf")
	}
	return sessionC, csrf
}

func TestHTTPProductCRUDFlow(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	body := `{"title":"T恤","slug":"tee","status":"active",
		"options":[{"name":"尺码","values":["S","M"]},{"name":"颜色","values":["黑","白"]}],
		"variants":[
			{"price_cents":9900,"quantity":10,"selections":[{"option":"尺码","value":"S"},{"option":"颜色","value":"黑"}]},
			{"price_cents":9900,"quantity":20,"selections":[{"option":"尺码","value":"M"},{"option":"颜色","value":"白"}]}
		]}`

	// 未登录 → 401
	if resp := doJSON(t, mux, "POST", "/admin/api/products", body, nil, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录建商品 = %d，期望 401", resp.StatusCode)
	}
	// 登录但无 CSRF → 403
	if resp := doJSON(t, mux, "POST", "/admin/api/products", body, auth, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("无 CSRF 建商品 = %d，期望 403", resp.StatusCode)
	}
	// 正常建商品 → 201
	resp := doJSON(t, mux, "POST", "/admin/api/products", body, auth, csrf)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建商品 = %d，期望 201；body=%s", resp.StatusCode, resp.Body)
	}
	var created struct {
		PublicID string `json:"public_id"`
	}
	_ = json.Unmarshal(resp.Body, &created)
	if created.PublicID == "" {
		t.Fatal("未返回 public_id")
	}

	// 列表含一条
	listResp := doJSON(t, mux, "GET", "/admin/api/products", "", auth, "")
	var list struct {
		Products []map[string]any `json:"products"`
	}
	_ = json.Unmarshal(listResp.Body, &list)
	if len(list.Products) != 1 {
		t.Fatalf("商品数 = %d，期望 1", len(list.Products))
	}

	// 取详情，拿一个变体 public_id
	getResp := doJSON(t, mux, "GET", "/admin/api/products/"+created.PublicID, "", auth, "")
	var detail struct {
		Variants []struct {
			PublicID string `json:"public_id"`
			Quantity int64  `json:"quantity"`
		} `json:"variants"`
	}
	_ = json.Unmarshal(getResp.Body, &detail)
	if len(detail.Variants) != 2 {
		t.Fatalf("详情变体数 = %d，期望 2", len(detail.Variants))
	}

	// 改库存
	vpid := detail.Variants[0].PublicID
	if resp := doJSON(t, mux, "PATCH", "/admin/api/variants/"+vpid+"/inventory", `{"quantity":999}`, auth, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("改库存 = %d，期望 200；body=%s", resp.StatusCode, resp.Body)
	}

	// 软删
	if resp := doJSON(t, mux, "DELETE", "/admin/api/products/"+created.PublicID, "", auth, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("软删 = %d，期望 200", resp.StatusCode)
	}
	// 软删后取 → 404
	if resp := doJSON(t, mux, "GET", "/admin/api/products/"+created.PublicID, "", auth, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("软删后取 = %d，期望 404", resp.StatusCode)
	}
}

func TestHTTPMediaUpload(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	// 建商品
	body := `{"title":"T","slug":"t","status":"active","options":[{"name":"尺码","values":["S"]}],"variants":[{"price_cents":1,"quantity":1,"selections":[{"option":"尺码","value":"S"}]}]}`
	resp := doJSON(t, mux, "POST", "/admin/api/products", body, auth, csrf)
	var created struct {
		PublicID string `json:"public_id"`
	}
	_ = json.Unmarshal(resp.Body, &created)

	// 造一张 PNG
	var img bytes.Buffer
	_ = png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 400, 300)))

	// multipart 上传
	var mb bytes.Buffer
	mw := multipart.NewWriter(&mb)
	fw, _ := mw.CreateFormFile("file", "x.png")
	_, _ = fw.Write(img.Bytes())
	_ = mw.Close()

	req := httptest.NewRequest("POST", "/admin/api/products/"+created.PublicID+"/media", &mb)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(csrfHeader, csrf)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("上传 = %d，期望 201；body=%s", res.StatusCode, b)
	}

	// 列表含 1 张
	lr := doJSON(t, mux, "GET", "/admin/api/products/"+created.PublicID+"/media", "", auth, "")
	var lst struct {
		Media []struct {
			PublicID    string `json:"public_id"`
			Derivatives []any  `json:"derivatives"`
		} `json:"media"`
	}
	_ = json.Unmarshal(lr.Body, &lst)
	if len(lst.Media) != 1 || len(lst.Media[0].Derivatives) == 0 {
		t.Fatalf("媒体列表异常: %+v", lst)
	}

	// 删除
	if dr := doJSON(t, mux, "DELETE", "/admin/api/media/"+lst.Media[0].PublicID, "", auth, csrf); dr.StatusCode != http.StatusOK {
		t.Fatalf("删图 = %d，期望 200", dr.StatusCode)
	}
}

func TestHTTPMarkets(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	resp := doJSON(t, mux, "GET", "/admin/api/markets", "", auth, "")
	var ml struct {
		Markets []struct {
			Code      string `json:"code"`
			Available bool   `json:"available"`
		} `json:"markets"`
	}
	_ = json.Unmarshal(resp.Body, &ml)
	var hasUS, hasSoon bool
	for _, m := range ml.Markets {
		if m.Code == "US" && m.Available {
			hasUS = true
		}
		if !m.Available {
			hasSoon = true
		}
	}
	if !hasUS || !hasSoon {
		t.Fatalf("市场列表异常: %+v", ml.Markets)
	}

	resp = doJSON(t, mux, "GET", "/admin/api/settings/market", "", auth, "")
	var cur struct {
		Configured bool `json:"configured"`
	}
	_ = json.Unmarshal(resp.Body, &cur)
	if cur.Configured {
		t.Fatal("初始应未配置市场")
	}

	if r := doJSON(t, mux, "PUT", "/admin/api/settings/market", `{"code":"MENA"}`, auth, csrf); r.StatusCode != http.StatusConflict {
		t.Fatalf("选即将上线市场 = %d，期望 409", r.StatusCode)
	}
	if r := doJSON(t, mux, "PUT", "/admin/api/settings/market", `{"code":"US"}`, auth, csrf); r.StatusCode != http.StatusOK {
		t.Fatalf("选美国 = %d，期望 200", r.StatusCode)
	}
	resp = doJSON(t, mux, "GET", "/admin/api/settings/market", "", auth, "")
	_ = json.Unmarshal(resp.Body, &cur)
	if !cur.Configured {
		t.Fatal("选定后应为已配置")
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

// ---- 域名步骤（M4.2.1）----

// TestValidateDomain 独立校验域名（后端自守：拒空/协议前缀/路径/空格/非法字符/无点，接受合法 FQDN）。
func TestValidateDomain(t *testing.T) {
	bad := []string{"", "   ", "http://shop.example.com", "https://shop.example.com",
		"shop.example.com/store", "shop example.com", "shop_example.com", "shop.example.com:8443", "localhost"}
	for _, in := range bad {
		if _, err := validateDomain(in); err == nil {
			t.Fatalf("非法域名 %q 应被拒", in)
		}
	}
	good := map[string]string{"shop.example.com": "shop.example.com", "  Shop.Example.com  ": "Shop.Example.com", "a-b.co": "a-b.co"}
	for in, want := range good {
		got, err := validateDomain(in)
		if err != nil || got != want {
			t.Fatalf("合法域名 %q 应通过并归一为 %q，得 (%q,%v)", in, want, got, err)
		}
	}
}

// TestWizardDomain 覆盖 DB 路径：needed→存域名(校验)→源 db→needed=false→跳过→CSRF。
func TestWizardDomain(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	// 未配、未跳过 → needed=true；来源 none、可签发（此实例 secure=false=dev，https_capable=false）。
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/domain", "", auth, ""); r.StatusCode != http.StatusOK || !bytes.Contains(r.Body, []byte(`"needed":true`)) {
		t.Fatalf("初始应 needed=true: %d %s", r.StatusCode, r.Body)
	}
	if r := doJSON(t, mux, "GET", "/admin/api/settings/domain", "", auth, ""); !bytes.Contains(r.Body, []byte(`"source":"none"`)) || !bytes.Contains(r.Body, []byte(`"https_capable":false`)) {
		t.Fatalf("初始 GET domain 应 source=none & https_capable=false(dev): %s", r.Body)
	}

	// 非法输入前后端双拦（后端独立）：空/协议前缀/路径/无点 → 400。
	for _, body := range []string{`{"domain":""}`, `{"domain":"http://shop.example.com"}`, `{"domain":"shop.example.com/x"}`, `{"domain":"localhost"}`} {
		if r := doJSON(t, mux, "PUT", "/admin/api/settings/domain", body, auth, csrf); r.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法域名 %s 应 400，得 %d %s", body, r.StatusCode, r.Body)
		}
	}

	// 存合法域名 → 200、来源 db。
	if r := doJSON(t, mux, "PUT", "/admin/api/settings/domain", `{"domain":"shop.example.com"}`, auth, csrf); r.StatusCode != http.StatusOK || !bytes.Contains(r.Body, []byte(`"source":"db"`)) || !bytes.Contains(r.Body, []byte(`"shop.example.com"`)) {
		t.Fatalf("存合法域名应 200 且 source=db: %d %s", r.StatusCode, r.Body)
	}
	// 已配 → needed=false。
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/domain", "", auth, ""); !bytes.Contains(r.Body, []byte(`"needed":false`)) {
		t.Fatalf("配好后应 needed=false: %s", r.Body)
	}
	// 存域名缺 CSRF → 403。
	if r := doJSON(t, mux, "PUT", "/admin/api/settings/domain", `{"domain":"a.example.com"}`, auth, ""); r.StatusCode != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应 403，得 %d", r.StatusCode)
	}
}

// TestWizardDomainSkip 覆盖留空跳过：skip→needed=false→skip 缺 CSRF→403。
func TestWizardDomainSkip(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	if r := doJSON(t, mux, "POST", "/admin/api/wizard/domain/skip", "", auth, csrf); r.StatusCode != http.StatusOK {
		t.Fatalf("skip 应 200: %d %s", r.StatusCode, r.Body)
	}
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/domain", "", auth, ""); !bytes.Contains(r.Body, []byte(`"needed":false`)) {
		t.Fatalf("跳过后应 needed=false: %s", r.Body)
	}
	if r := doJSON(t, mux, "POST", "/admin/api/wizard/domain/skip", "", auth, ""); r.StatusCode != http.StatusForbidden {
		t.Fatalf("skip 缺 CSRF 应 403，得 %d", r.StatusCode)
	}
}

// TestWizardDomainEnvReadonly 覆盖 env 覆盖态：源 env、只读、PUT→409、向导 needed=false（决策 C）。
func TestWizardDomainEnvReadonly(t *testing.T) {
	_, mux := newHTTPEnvDomain(t, "env.example.com")
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	if r := doJSON(t, mux, "GET", "/admin/api/settings/domain", "", auth, ""); !bytes.Contains(r.Body, []byte(`"source":"env"`)) || !bytes.Contains(r.Body, []byte(`"readonly":true`)) || !bytes.Contains(r.Body, []byte(`"env.example.com"`)) {
		t.Fatalf("env 覆盖应 source=env & readonly=true: %s", r.Body)
	}
	// env 覆盖时写入被拒（只读，不双写 DB）→ 409。
	if r := doJSON(t, mux, "PUT", "/admin/api/settings/domain", `{"domain":"other.example.com"}`, auth, csrf); r.StatusCode != http.StatusConflict {
		t.Fatalf("env 只读应 409，得 %d %s", r.StatusCode, r.Body)
	}
	// env 已提供域名 → 向导不再需要域名步骤。
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/domain", "", auth, ""); !bytes.Contains(r.Body, []byte(`"needed":false`)) {
		t.Fatalf("env 已配应 needed=false: %s", r.Body)
	}
}
