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
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

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

// ---- HTTP 层 ----

func newHTTP(t *testing.T) (*HTTP, http.Handler) {
	svc := newSvc(t)
	root := t.TempDir() + "/media"
	md := media.New(svc.db, media.NewLocalBackend(root), media.NewDefaultPolicy(root, 10<<20, 0), 20)
	h := NewHTTP(svc, catalog.New(svc.db), md, settings.New(svc.db), false)
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
