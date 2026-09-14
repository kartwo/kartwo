// Admin 鉴权测试 / Admin Auth Tests
// 功能：初始化幂等、登录校验、会话鉴权、登出、CSRF、登录限流（核心安全逻辑必须单测）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"archive/zip"
	"bytes"
	"context"
	cryptotls "crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/auth"
	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/importer"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/redirect"
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

func TestUpdateOwnerCredentialsReencryptsSettingsAndInvalidatesSessions(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.Initialize(ctx, "admin", "old-password"); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	s1, err := svc.Login(ctx, "admin", "old-password")
	if err != nil {
		t.Fatalf("首次登录失败: %v", err)
	}
	s2, err := svc.Login(ctx, "admin", "old-password")
	if err != nil {
		t.Fatalf("第二次登录失败: %v", err)
	}
	oldKEK, ok := svc.Key(s1.Token)
	if !ok {
		t.Fatal("旧会话应持有 KEK")
	}
	oldKEKCopy := append([]byte(nil), oldKEK...)
	settingsSvc := settings.New(svc.db)
	if err := settingsSvc.SetEncrypted(ctx, "test.secret.one", []byte("secret-value"), oldKEK); err != nil {
		t.Fatalf("写入加密设置失败: %v", err)
	}
	ac, err := svc.Authenticate(ctx, s1.Token)
	if err != nil {
		t.Fatalf("读取当前身份失败: %v", err)
	}
	changed, err := svc.UpdateOwnerCredentials(ctx, ac.AdminID, "new-admin", "old-password", "new-password")
	if err != nil || !changed {
		t.Fatalf("同时修改用户名和密码失败: changed=%v err=%v", changed, err)
	}
	for _, token := range []string{s1.Token, s2.Token} {
		if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("更新后旧会话必须失效: %v", err)
		}
		if _, ok := svc.Key(token); ok {
			t.Fatal("更新后内存金库不得保留旧会话 KEK")
		}
	}
	if _, err := svc.Login(ctx, "admin", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("旧用户名不得继续登录: %v", err)
	}
	if _, err := svc.Login(ctx, "new-admin", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("旧密码不得继续登录: %v", err)
	}
	newSession, err := svc.Login(ctx, "new-admin", "new-password")
	if err != nil {
		t.Fatalf("新凭据登录失败: %v", err)
	}
	newKEK, ok := svc.Key(newSession.Token)
	if !ok {
		t.Fatal("新会话应持有新 KEK")
	}
	plaintext, err := settingsSvc.GetEncrypted(ctx, "test.secret.one", newKEK)
	if err != nil || string(plaintext) != "secret-value" {
		t.Fatalf("新密码应能解密原配置: plaintext=%q err=%v", plaintext, err)
	}
	if _, err := settingsSvc.GetEncrypted(ctx, "test.secret.one", oldKEKCopy); err == nil {
		t.Fatal("旧密码派生的 KEK 不应再能解密配置")
	}
}

func TestUpdateOwnerCredentialsRollsBackWhenEncryptedSettingIsDamaged(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.Initialize(ctx, "admin", "old-password"); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	session, err := svc.Login(ctx, "admin", "old-password")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	oldKEK, ok := svc.Key(session.Token)
	if !ok {
		t.Fatal("旧会话应持有 KEK")
	}
	settingsSvc := settings.New(svc.db)
	if err := settingsSvc.SetEncrypted(ctx, "test.secret.valid", []byte("still-safe"), oldKEK); err != nil {
		t.Fatalf("写入有效加密设置失败: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO setting (key, value, encrypted) VALUES (?, ?, 1)`, "test.secret.damaged", "not-ciphertext"); err != nil {
		t.Fatalf("构造损坏密文失败: %v", err)
	}
	ac, err := svc.Authenticate(ctx, session.Token)
	if err != nil {
		t.Fatalf("读取当前身份失败: %v", err)
	}
	if _, err := svc.UpdateOwnerCredentials(ctx, ac.AdminID, "new-admin", "old-password", "new-password"); err == nil {
		t.Fatal("存在损坏密文时必须拒绝并回滚账号更新")
	}
	if _, err := svc.Authenticate(ctx, session.Token); err != nil {
		t.Fatalf("回滚后原会话仍应有效: %v", err)
	}
	if _, err := svc.Login(ctx, "admin", "old-password"); err != nil {
		t.Fatalf("回滚后原凭据仍应有效: %v", err)
	}
	if _, err := svc.Login(ctx, "new-admin", "new-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("回滚后新凭据不得生效: %v", err)
	}
	plaintext, err := settingsSvc.GetEncrypted(ctx, "test.secret.valid", oldKEK)
	if err != nil || string(plaintext) != "still-safe" {
		t.Fatalf("回滚后原加密配置必须保持可读: plaintext=%q err=%v", plaintext, err)
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
func newHTTPEnvDomain(t *testing.T, envDomain string, envShopName ...string) (*HTTP, http.Handler) {
	svc := newSvc(t)
	dataDir := t.TempDir()
	root := dataDir + "/media"
	md := media.New(svc.db, media.NewLocalBackend(root), media.NewDefaultPolicy(root, 10<<20, 0), 20)
	set := settings.New(svc.db)
	ord := order.New(svc.db, set)
	mc := mail.NewCache(set)
	svc.SetMailKeys(mc)
	cat := catalog.New(svc.db)
	h := NewHTTP(svc, cat, importer.New(svc.db, cat, md, redirect.New(svc.db)), md, set, ord, nil, mc, backup.New(svc.db, dataDir, "test-version"), BackupConfig{Interval: 24 * time.Hour, Retention: 7}, envDomain, false, nil, envShopName...)
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func TestHTTPShopNameRemainsEditableWithEnvFallbackAndLogoLifecycle(t *testing.T) {
	_, mux := newHTTPEnvDomain(t, "", "M4 Final Store")
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	initial := doJSON(t, mux, "GET", "/admin/api/settings/shop", "", auth, "")
	if initial.StatusCode != http.StatusOK || !bytes.Contains(initial.Body, []byte(`"name":"M4 Final Store"`)) || !bytes.Contains(initial.Body, []byte(`"readonly":false`)) {
		t.Fatalf("环境变量应只作可编辑回退: %d %s", initial.StatusCode, initial.Body)
	}
	saved := doJSON(t, mux, "PUT", "/admin/api/settings/shop", `{"name":"Kartwo Running"}`, auth, csrf)
	if saved.StatusCode != http.StatusOK || !bytes.Contains(saved.Body, []byte(`"name":"Kartwo Running"`)) {
		t.Fatalf("后台店名应可覆盖环境回退: %d %s", saved.StatusCode, saved.Body)
	}

	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, image.NewRGBA(image.Rect(0, 0, 1200, 400))); err != nil {
		t.Fatal(err)
	}
	var multipartBody bytes.Buffer
	mw := multipart.NewWriter(&multipartBody)
	part, err := mw.CreateFormFile("file", "kartwo-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(imageBody.Bytes())
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/admin/api/settings/shop/logo", &multipartBody)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(csrfHeader, csrf)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"logo_url":"/media/brand/`)) {
		t.Fatalf("上传 Logo 失败: %d %s", rec.Code, rec.Body.String())
	}

	deleted := doJSON(t, mux, "DELETE", "/admin/api/settings/shop/logo", "", auth, csrf)
	if deleted.StatusCode != http.StatusOK || !bytes.Contains(deleted.Body, []byte(`"logo_url":""`)) {
		t.Fatalf("删除 Logo 失败: %d %s", deleted.StatusCode, deleted.Body)
	}
}

func TestHTTPUpdateAccountSignsOutAndAudits(t *testing.T) {
	_, mux := newHTTP(t)
	session, csrf := loginAndCookies(t, mux)
	cookies := []*http.Cookie{session}

	account := doJSON(t, mux, http.MethodGet, "/admin/api/account", "", cookies, "")
	if account.StatusCode != http.StatusOK || !bytes.Contains(account.Body, []byte(`"username":"admin"`)) || !bytes.Contains(account.Body, []byte(`"readonly":false`)) {
		t.Fatalf("读取账号设置失败: %d %s", account.StatusCode, account.Body)
	}
	wrong := doJSON(t, mux, http.MethodPut, "/admin/api/account", `{"username":"new-admin","current_password":"wrong","new_password":"new-password"}`, cookies, csrf)
	if wrong.StatusCode != http.StatusForbidden {
		t.Fatalf("当前密码错误应拒绝: %d %s", wrong.StatusCode, wrong.Body)
	}
	updated := doJSON(t, mux, http.MethodPut, "/admin/api/account", `{"username":"new-admin","current_password":"supersecret","new_password":"new-password"}`, cookies, csrf)
	if updated.StatusCode != http.StatusOK || !bytes.Contains(updated.Body, []byte(`"signed_out":true`)) || bytes.Contains(updated.Body, []byte("new-password")) {
		t.Fatalf("更新账号响应异常: %d %s", updated.StatusCode, updated.Body)
	}
	clearedSession := false
	clearedCSRF := false
	for _, cookie := range updated.Cookies {
		if cookie.Name == sessionCookie && cookie.MaxAge < 0 {
			clearedSession = true
		}
		if cookie.Name == csrfCookie && cookie.MaxAge < 0 {
			clearedCSRF = true
		}
	}
	if !clearedSession || !clearedCSRF {
		t.Fatalf("更新账号后必须清除登录 Cookie: session=%v csrf=%v", clearedSession, clearedCSRF)
	}
	if me := doJSON(t, mux, http.MethodGet, "/admin/api/me", "", cookies, ""); me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("更新账号后旧会话应失效: %d %s", me.StatusCode, me.Body)
	}
	login := doJSON(t, mux, http.MethodPost, "/admin/api/login", `{"username":"new-admin","password":"new-password"}`, nil, "")
	if login.StatusCode != http.StatusOK {
		t.Fatalf("新凭据登录失败: %d %s", login.StatusCode, login.Body)
	}
	audit := doJSON(t, mux, http.MethodGet, "/admin/api/audit-events", "", login.Cookies, "")
	if audit.StatusCode != http.StatusOK || !bytes.Contains(audit.Body, []byte(`"action":"account.credentials_update"`)) {
		t.Fatalf("账号更新应留下审计记录: %d %s", audit.StatusCode, audit.Body)
	}
}

func TestHTTPExportData(t *testing.T) {
	_, mux := newHTTP(t)
	if resp := doJSON(t, mux, "GET", "/admin/api/export", "", nil, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录导出应 401，得 %d", resp.StatusCode)
	}
	doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")
	login := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
	resp := doJSON(t, mux, "GET", "/admin/api/export", "", login.Cookies, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("导出应 200，得 %d %s", resp.StatusCode, resp.Body)
	}
	audit := doJSON(t, mux, "GET", "/admin/api/audit-events", "", login.Cookies, "")
	if audit.StatusCode != http.StatusOK || !bytes.Contains(audit.Body, []byte(`"action":"export.create"`)) || !bytes.Contains(audit.Body, []byte(`"target_public_id":"shop-data"`)) {
		t.Fatalf("成功生成导出包应留审计事件: %d %s", audit.StatusCode, audit.Body)
	}
	reader, err := zip.NewReader(bytes.NewReader(resp.Body), int64(len(resp.Body)))
	if err != nil {
		t.Fatalf("响应应为 ZIP: %v", err)
	}
	entries := map[string]bool{}
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	if !entries["manifest.json"] || !entries["shop.db"] {
		t.Fatalf("导出 ZIP 缺少核心文件: %+v", entries)
	}
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

func TestHTTPContentPageCreateAndUpdatePayload(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	// 严格解码仍应拒绝界面状态字段；前端必须只发送 API 定义的字段。
	unknown := `{"public_id":"","title":"About Us","slug":"aboutus","body_markdown":"kartwo.com","seo_description":"about kartwo.com","status":"draft"}`
	if resp := doJSON(t, mux, http.MethodPost, "/admin/api/content-pages", unknown, auth, csrf); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知 public_id 应拒绝，得 %d %s", resp.StatusCode, resp.Body)
	}

	valid := `{"title":"About Us","slug":"aboutus","body_markdown":"kartwo.com","seo_description":"about kartwo.com","status":"draft"}`
	created := doJSON(t, mux, http.MethodPost, "/admin/api/content-pages", valid, auth, csrf)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("合法内容页应创建，得 %d %s", created.StatusCode, created.Body)
	}
	var result struct {
		PublicID string `json:"public_id"`
	}
	if err := json.Unmarshal(created.Body, &result); err != nil || result.PublicID == "" {
		t.Fatalf("创建响应异常: %s", created.Body)
	}
	updated := `{"title":"About Kartwo","slug":"aboutus","body_markdown":"# About\nUpdated.","seo_description":"About Kartwo","status":"active"}`
	if resp := doJSON(t, mux, http.MethodPatch, "/admin/api/content-pages/"+result.PublicID, updated, auth, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("合法内容页应更新，得 %d %s", resp.StatusCode, resp.Body)
	}
	page := doJSON(t, mux, http.MethodGet, "/admin/api/content-pages/"+result.PublicID, "", auth, "")
	if page.StatusCode != http.StatusOK || !bytes.Contains(page.Body, []byte(`"title":"About Kartwo"`)) || !bytes.Contains(page.Body, []byte(`"status":"active"`)) {
		t.Fatalf("内容页更新未生效: %d %s", page.StatusCode, page.Body)
	}
}

func TestHTTPPolicyProfileAndFooterGeneration(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	if r := doJSON(t, mux, http.MethodPost, "/admin/api/content-pages/generate-footer", `{"publish":true,"overwrite":false}`, auth, csrf); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未保存资料不应生成，得 %d %s", r.StatusCode, r.Body)
	}
	if r := doJSON(t, mux, http.MethodPut, "/admin/api/settings/policy-profile", `{"support_email":"bad"}`, auth, csrf); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法资料应拒绝，得 %d %s", r.StatusCode, r.Body)
	}
	body := `{"support_email":"info@kartwo.com","ship_from":"China","processing_hours":48,"return_window_days":7,"return_shipping_payer":"buyer","business_name":"kartwo.com","delivery_estimates":[{"region":"Asia","min_business_days":5,"max_business_days":10},{"region":"Europe","min_business_days":7,"max_business_days":15},{"region":"North America","min_business_days":7,"max_business_days":15},{"region":"South America","min_business_days":10,"max_business_days":25},{"region":"Africa","min_business_days":10,"max_business_days":25},{"region":"Oceania","min_business_days":7,"max_business_days":15},{"region":"Antarctica","min_business_days":20,"max_business_days":35}]}`
	if r := doJSON(t, mux, http.MethodPut, "/admin/api/settings/policy-profile", body, auth, csrf); r.StatusCode != http.StatusOK {
		t.Fatalf("合法资料应保存，得 %d %s", r.StatusCode, r.Body)
	}
	if r := doJSON(t, mux, http.MethodGet, "/admin/api/settings/policy-profile", "", auth, ""); r.StatusCode != http.StatusOK || !bytes.Contains(r.Body, []byte(`"configured":true`)) || !bytes.Contains(r.Body, []byte(`info@kartwo.com`)) {
		t.Fatalf("应能读取已保存资料，得 %d %s", r.StatusCode, r.Body)
	}
	generated := doJSON(t, mux, http.MethodPost, "/admin/api/content-pages/generate-footer", `{"publish":true,"overwrite":false}`, auth, csrf)
	if generated.StatusCode != http.StatusOK || !bytes.Contains(generated.Body, []byte(`"created":7`)) {
		t.Fatalf("应生成七个页脚页面，得 %d %s", generated.StatusCode, generated.Body)
	}
	listed := doJSON(t, mux, http.MethodGet, "/admin/api/content-pages", "", auth, "")
	if listed.StatusCode != http.StatusOK || !bytes.Contains(listed.Body, []byte(`"slug":"shipping-policy"`)) || !bytes.Contains(listed.Body, []byte(`"status":"active"`)) {
		t.Fatalf("生成页面应已发布，得 %d %s", listed.StatusCode, listed.Body)
	}
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

func TestHTTPAuditEvents(t *testing.T) {
	_, mux := newHTTP(t)
	doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")
	login := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
	resp := doJSON(t, mux, "GET", "/admin/api/audit-events", "", login.Cookies, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("审计列表应 200，得 %d %s", resp.StatusCode, resp.Body)
	}
	if !bytes.Contains(resp.Body, []byte(`"action":"admin.login"`)) || bytes.Contains(resp.Body, []byte("supersecret")) {
		t.Fatalf("应记录登录且绝不泄露口令: %s", resp.Body)
	}
}

type fakeRefundService struct{ calls int }

func (s *fakeRefundService) Refund(_ context.Context, _ string) error {
	s.calls++
	if s.calls > 1 {
		return payment.ErrNotRefundable
	}
	return nil
}

// TestHTTPRefundAuditOnlyAfterSuccess 确保退款审计只记录已成功完成的退款，重复退款不追加伪成功事件。
func TestHTTPRefundAuditOnlyAfterSuccess(t *testing.T) {
	h, mux := newHTTP(t)
	fakePay := &fakeRefundService{}
	h.pay = fakePay
	sess, csrf := loginAndCookies(t, mux)

	const orderID = "ord-refund-audit-001"
	if ok := doJSON(t, mux, http.MethodPost, "/admin/api/orders/"+orderID+"/refund", "", []*http.Cookie{sess}, csrf); ok.StatusCode != http.StatusOK {
		t.Fatalf("首次退款应 200，得 %d %s", ok.StatusCode, ok.Body)
	}
	if fakePay.calls != 1 {
		t.Fatalf("退款服务调用次数 = %d，期望 1", fakePay.calls)
	}

	audit := doJSON(t, mux, http.MethodGet, "/admin/api/audit-events", "", []*http.Cookie{sess}, "")
	if audit.StatusCode != http.StatusOK || !bytes.Contains(audit.Body, []byte(`"action":"order.refund"`)) || !bytes.Contains(audit.Body, []byte(orderID)) {
		t.Fatalf("成功退款应留审计事件: %d %s", audit.StatusCode, audit.Body)
	}
	if n := bytes.Count(audit.Body, []byte(`"action":"order.refund"`)); n != 1 {
		t.Fatalf("成功退款审计事件数 = %d，期望 1: %s", n, audit.Body)
	}

	if repeated := doJSON(t, mux, http.MethodPost, "/admin/api/orders/"+orderID+"/refund", "", []*http.Cookie{sess}, csrf); repeated.StatusCode != http.StatusConflict {
		t.Fatalf("重复退款应 409，得 %d %s", repeated.StatusCode, repeated.Body)
	}
	audit = doJSON(t, mux, http.MethodGet, "/admin/api/audit-events", "", []*http.Cookie{sess}, "")
	if n := bytes.Count(audit.Body, []byte(`"action":"order.refund"`)); n != 1 {
		t.Fatalf("失败重复退款不应追加审计事件，得 %d: %s", n, audit.Body)
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

func TestHTTPCSVImport(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	const csvText = "title,slug,status,description,option1_name,option1_value,option2_name,option2_value,sku,price_cents,quantity\n帽子,cap,active,,尺寸,均码,,,CAP-ONE,2900,8\n"
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, err := w.CreateFormFile("file", "products.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(csvText)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	request := func(csrfValue string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/imports/csv/preview", &body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.AddCookie(sess)
		if csrfValue != "" {
			req.Header.Set(csrfHeader, csrfValue)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(""); rec.Code != http.StatusForbidden {
		t.Fatalf("导入预览缺 CSRF = %d", rec.Code)
	}
	// multipart body 在前一请求已读完，重建带 CSRF 的请求。
	var body2 bytes.Buffer
	w2 := multipart.NewWriter(&body2)
	f2, _ := w2.CreateFormFile("file", "products.csv")
	_, _ = f2.Write([]byte(csvText))
	_ = w2.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/imports/csv/preview", &body2)
	req.Header.Set("Content-Type", w2.FormDataContentType())
	req.AddCookie(sess)
	req.Header.Set(csrfHeader, csrf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("导入预览 = %d %s", rec.Code, rec.Body.String())
	}
	var preview struct {
		PublicID     string `json:"public_id"`
		ProductCount int    `json:"product_count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &preview)
	if preview.PublicID == "" || preview.ProductCount != 1 {
		t.Fatalf("预览异常: %s", rec.Body.String())
	}
	if list := doJSON(t, mux, "GET", "/admin/api/products", "", []*http.Cookie{sess}, ""); bytes.Contains(list.Body, []byte("cap")) {
		t.Fatal("预览不应写商品")
	}
	if bad := doJSON(t, mux, "POST", "/admin/api/imports/"+preview.PublicID+"/execute", "", []*http.Cookie{sess}, ""); bad.StatusCode != http.StatusForbidden {
		t.Fatalf("确认导入缺 CSRF = %d", bad.StatusCode)
	}
	if ok := doJSON(t, mux, "POST", "/admin/api/imports/"+preview.PublicID+"/execute", "", []*http.Cookie{sess}, csrf); ok.StatusCode != http.StatusOK {
		t.Fatalf("确认导入 = %d %s", ok.StatusCode, ok.Body)
	}
	if list := doJSON(t, mux, "GET", "/admin/api/products", "", []*http.Cookie{sess}, ""); !bytes.Contains(list.Body, []byte("cap")) {
		t.Fatal("确认导入后应有商品")
	}
	if audit := doJSON(t, mux, "GET", "/admin/api/audit-events", "", []*http.Cookie{sess}, ""); !bytes.Contains(audit.Body, []byte(`"action":"import.execute"`)) || !bytes.Contains(audit.Body, []byte(preview.PublicID)) {
		t.Fatalf("确认导入应留审计事件: %s", audit.Body)
	}
}

func TestHTTPAuditKeySettings(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	backup := doJSON(t, mux, http.MethodPut, "/admin/api/settings/backup", `{"interval":"90m","retention":"12"}`, []*http.Cookie{sess}, csrf)
	if backup.StatusCode != http.StatusOK {
		t.Fatalf("保存备份设置应 200，得 %d %s", backup.StatusCode, backup.Body)
	}
	payment := doJSON(t, mux, http.MethodPut, "/admin/api/settings/payment", `{"stripe":{"mode":"test","publishable":"pk_test_public","secret":"sk_test_secret","webhook_secret":"whsec_test_secret"}}`, []*http.Cookie{sess}, csrf)
	if payment.StatusCode != http.StatusOK {
		t.Fatalf("保存收款设置应 200，得 %d %s", payment.StatusCode, payment.Body)
	}
	audit := doJSON(t, mux, http.MethodGet, "/admin/api/audit-events", "", []*http.Cookie{sess}, "")
	if audit.StatusCode != http.StatusOK || !bytes.Contains(audit.Body, []byte(`"action":"backup.settings_update"`)) || !bytes.Contains(audit.Body, []byte(`"action":"payment.settings_update"`)) {
		t.Fatalf("关键设置应留审计事件: %d %s", audit.StatusCode, audit.Body)
	}
	if bytes.Contains(audit.Body, []byte("sk_test_secret")) || bytes.Contains(audit.Body, []byte("whsec_test_secret")) {
		t.Fatalf("审计事件绝不能泄露收款密钥: %s", audit.Body)
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

func TestPublicDemoSessionEnforcesOwnershipQuotaAndOwnerOnlyRoutes(t *testing.T) {
	h, mux := newHTTP(t)
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: 45 * time.Minute, MaxProducts: 3, MaxImageBytes: 2 << 20, CleanupInterval: 5 * time.Minute})
	ownerSession, ownerCSRF := loginAndCookies(t, mux)
	owner := []*http.Cookie{ownerSession}

	baselineBody := `{"title":"Baseline","slug":"baseline","status":"active","options":[{"name":"Style","values":["One"]}],"variants":[{"sku":"BASE-1","price_cents":1000,"quantity":8,"selections":[{"option":"Style","value":"One"}]}]}`
	baseline := doJSON(t, mux, http.MethodPost, "/admin/api/products", baselineBody, owner, ownerCSRF)
	if baseline.StatusCode != http.StatusCreated {
		t.Fatalf("创建基线商品失败: %d %s", baseline.StatusCode, baseline.Body)
	}
	var baselineResult struct {
		PublicID string `json:"public_id"`
	}
	_ = json.Unmarshal(baseline.Body, &baselineResult)

	demoLogin := doJSON(t, mux, http.MethodPost, "/admin/api/demo-session", "", nil, "")
	if demoLogin.StatusCode != http.StatusCreated {
		t.Fatalf("一键演示登录失败: %d %s", demoLogin.StatusCode, demoLogin.Body)
	}
	var demoSession *http.Cookie
	var demoCSRF string
	for _, cookie := range demoLogin.Cookies {
		if cookie.Name == sessionCookie {
			demoSession = cookie
		}
		if cookie.Name == csrfCookie {
			demoCSRF = cookie.Value
		}
	}
	demo := []*http.Cookie{demoSession}
	if me := doJSON(t, mux, http.MethodGet, "/admin/api/me", "", demo, ""); me.StatusCode != http.StatusOK || !bytes.Contains(me.Body, []byte(`"role":"demo"`)) {
		t.Fatalf("演示身份异常: %d %s", me.StatusCode, me.Body)
	}
	if err := h.settings.SetPlain(context.Background(), payment.KeyStripePublishable, "pk_test_must_not_leak"); err != nil {
		t.Fatal(err)
	}
	if err := h.settings.SetPlain(context.Background(), payment.KeyPayPalClientID, "paypal-client-must-not-leak"); err != nil {
		t.Fatal(err)
	}
	paymentView := doJSON(t, mux, http.MethodGet, "/admin/api/settings/payment", "", demo, "")
	if paymentView.StatusCode != http.StatusOK || !bytes.Contains(paymentView.Body, []byte(`"readonly":true`)) {
		t.Fatalf("演示身份应能只读查看脱敏收款配置: %d %s", paymentView.StatusCode, paymentView.Body)
	}
	if bytes.Contains(paymentView.Body, []byte("must_not_leak")) || bytes.Contains(paymentView.Body, []byte("must-not-leak")) {
		t.Fatalf("演示身份不应看到支付客户端标识: %s", paymentView.Body)
	}
	if _, err := h.svc.db.Exec(`INSERT INTO customer (public_id, email) VALUES ('demo-customer', 'private@example.com')`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.db.Exec(`INSERT INTO "order" (public_id, customer_id, status, email, ship_name, ship_phone, ship_address, ship_country, currency, subtotal_cents, total_cents)
		VALUES ('demo-order-sensitive', (SELECT id FROM customer WHERE public_id='demo-customer'), 'pending', 'private@example.com', 'Private Name', '123456789', 'Private Address', 'US', 'USD', 1000, 1000)`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/admin/api/diagnostics", "/admin/api/audit-events", "/admin/api/orders", "/admin/api/account", "/admin/api/settings/shop", "/admin/api/settings/domain", "/admin/api/settings/translation", "/admin/api/settings/shipping/countries", "/admin/api/settings/shipping"} {
		view := doJSON(t, mux, http.MethodGet, path, "", demo, "")
		if view.StatusCode != http.StatusOK {
			t.Fatalf("演示身份只读访问 %s 应成功: %d %s", path, view.StatusCode, view.Body)
		}
		if bytes.Contains(view.Body, []byte("private@example.com")) || bytes.Contains(view.Body, []byte("Private Name")) || bytes.Contains(view.Body, []byte("Private Address")) {
			t.Fatalf("演示只读接口 %s 泄露订单隐私: %s", path, view.Body)
		}
	}
	orderView := doJSON(t, mux, http.MethodGet, "/admin/api/orders/demo-order-sensitive", "", demo, "")
	if orderView.StatusCode != http.StatusOK || !bytes.Contains(orderView.Body, []byte("已隐藏")) {
		t.Fatalf("演示订单详情应可查看且完成脱敏: %d %s", orderView.StatusCode, orderView.Body)
	}
	for _, private := range []string{"private@example.com", "Private Name", "123456789", "Private Address"} {
		if bytes.Contains(orderView.Body, []byte(private)) {
			t.Fatalf("演示订单详情泄露隐私 %q: %s", private, orderView.Body)
		}
	}
	if denied := doJSON(t, mux, http.MethodPut, "/admin/api/settings/shop", `{"name":"Attacked"}`, demo, demoCSRF); denied.StatusCode != http.StatusForbidden {
		t.Fatalf("演示身份修改设置应 403: %d %s", denied.StatusCode, denied.Body)
	}
	if denied := doJSON(t, mux, http.MethodPut, "/admin/api/account", `{"username":"attacker","current_password":"x","new_password":"new-password"}`, demo, demoCSRF); denied.StatusCode != http.StatusForbidden {
		t.Fatalf("演示身份修改管理员账号应 403: %d %s", denied.StatusCode, denied.Body)
	}
	if denied := doJSON(t, mux, http.MethodGet, "/admin/api/export", "", demo, ""); denied.StatusCode != http.StatusForbidden {
		t.Fatalf("演示身份执行数据导出应 403: %d %s", denied.StatusCode, denied.Body)
	}
	update := `{"title":"Attacked","title_zh":"","description":"","seo_description":"","seo_description_zh":"","status":"draft","category_public_ids":[]}`
	if denied := doJSON(t, mux, http.MethodPatch, "/admin/api/products/"+baselineResult.PublicID, update, demo, demoCSRF); denied.StatusCode != http.StatusForbidden {
		t.Fatalf("演示身份修改基线商品应 403: %d %s", denied.StatusCode, denied.Body)
	}

	for i := 1; i <= 3; i++ {
		body := fmt.Sprintf(`{"title":"Demo %d","slug":"demo-%d","status":"active","options":[{"name":"Style","values":["One"]}],"variants":[{"sku":"DEMO-%d","price_cents":1000,"quantity":1,"selections":[{"option":"Style","value":"One"}]}]}`, i, i, i)
		created := doJSON(t, mux, http.MethodPost, "/admin/api/products", body, demo, demoCSRF)
		if created.StatusCode != http.StatusCreated {
			t.Fatalf("第 %d 个临时商品创建失败: %d %s", i, created.StatusCode, created.Body)
		}
	}
	fourth := doJSON(t, mux, http.MethodPost, "/admin/api/products", `{"title":"Demo 4","slug":"demo-4","status":"draft","options":[{"name":"Style","values":["One"]}],"variants":[{"sku":"DEMO-4","price_cents":1000,"quantity":1,"selections":[{"option":"Style","value":"One"}]}]}`, demo, demoCSRF)
	if fourth.StatusCode != http.StatusConflict {
		t.Fatalf("第四个临时商品应被配额拒绝: %d %s", fourth.StatusCode, fourth.Body)
	}
	list := doJSON(t, mux, http.MethodGet, "/admin/api/products", "", demo, "")
	if !bytes.Contains(list.Body, []byte(`"slug":"demo-1"`)) || !bytes.Contains(list.Body, []byte(`"status":"draft"`)) || !bytes.Contains(list.Body, []byte(`"demo_owned":true`)) {
		t.Fatalf("演示商品应强制草稿并标注归属: %s", list.Body)
	}
	otherLogin := doJSON(t, mux, http.MethodPost, "/admin/api/demo-session", "", nil, "")
	var otherSession *http.Cookie
	for _, cookie := range otherLogin.Cookies {
		if cookie.Name == sessionCookie {
			otherSession = cookie
		}
	}
	otherList := doJSON(t, mux, http.MethodGet, "/admin/api/products", "", []*http.Cookie{otherSession}, "")
	if bytes.Contains(otherList.Body, []byte(`"slug":"demo-1"`)) || !bytes.Contains(otherList.Body, []byte(`"slug":"baseline"`)) {
		t.Fatalf("不同演示会话必须隔离临时商品，同时都能看基线商品: %s", otherList.Body)
	}
	reset := doJSON(t, mux, http.MethodPost, "/admin/api/demo/reset", "", demo, demoCSRF)
	if reset.StatusCode != http.StatusOK || !bytes.Contains(reset.Body, []byte(`"removed_products":3`)) {
		t.Fatalf("演示重置失败: %d %s", reset.StatusCode, reset.Body)
	}
	if got, err := h.cat.GetProduct(context.Background(), baselineResult.PublicID); err != nil || got.Title != "Baseline" {
		t.Fatalf("重置不能影响基线商品: product=%+v err=%v", got, err)
	}
}

func TestDemoAccountCannotUsePasswordLoginOrUnlockKEK(t *testing.T) {
	svc := newSvc(t)
	if _, err := svc.CreateDemoSession(context.Background(), 45*time.Minute); err != nil {
		t.Fatalf("创建演示会话失败: %v", err)
	}
	if _, err := svc.Login(context.Background(), demoUsername, "anything"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("演示账号不应支持密码登录: %v", err)
	}
}

func TestPublicDemoAllowsOneSmallImageOnly(t *testing.T) {
	h, mux := newHTTP(t)
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: 45 * time.Minute, MaxProducts: 3, MaxImageBytes: 2 << 20, CleanupInterval: 5 * time.Minute})
	_, _ = loginAndCookies(t, mux)
	demoLogin := doJSON(t, mux, http.MethodPost, "/admin/api/demo-session", "", nil, "")
	var session *http.Cookie
	var csrf string
	for _, cookie := range demoLogin.Cookies {
		if cookie.Name == sessionCookie {
			session = cookie
		}
		if cookie.Name == csrfCookie {
			csrf = cookie.Value
		}
	}
	body := `{"title":"Image Demo","slug":"image-demo","status":"active","options":[{"name":"Style","values":["One"]}],"variants":[{"sku":"IMG-DEMO","price_cents":1000,"quantity":1,"selections":[{"option":"Style","value":"One"}]}]}`
	created := doJSON(t, mux, http.MethodPost, "/admin/api/products", body, []*http.Cookie{session}, csrf)
	var result struct {
		PublicID string `json:"public_id"`
	}
	_ = json.Unmarshal(created.Body, &result)

	var imageBody bytes.Buffer
	_ = png.Encode(&imageBody, image.NewRGBA(image.Rect(0, 0, 100, 100)))
	upload := func() int {
		var multipartBody bytes.Buffer
		writer := multipart.NewWriter(&multipartBody)
		part, _ := writer.CreateFormFile("file", "demo.png")
		_, _ = part.Write(imageBody.Bytes())
		_ = writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/admin/api/products/"+result.PublicID+"/media", &multipartBody)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set(csrfHeader, csrf)
		req.AddCookie(session)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := upload(); code != http.StatusCreated {
		t.Fatalf("首张演示图片应允许上传，得 %d", code)
	}
	if code := upload(); code != http.StatusConflict {
		t.Fatalf("第二张演示图片应被拒绝，得 %d", code)
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
	mediaID := lst.Media[0].PublicID
	audit := doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, "")
	if audit.StatusCode != http.StatusOK || !bytes.Contains(audit.Body, []byte(`"action":"media.upload"`)) || !bytes.Contains(audit.Body, []byte(mediaID)) {
		t.Fatalf("上传成功应留媒体审计事件: %d %s", audit.StatusCode, audit.Body)
	}

	// 删除
	if dr := doJSON(t, mux, "DELETE", "/admin/api/media/"+mediaID, "", auth, csrf); dr.StatusCode != http.StatusOK {
		t.Fatalf("删图 = %d，期望 200", dr.StatusCode)
	}
	audit = doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, "")
	if audit.StatusCode != http.StatusOK || !bytes.Contains(audit.Body, []byte(`"action":"media.delete"`)) || !bytes.Contains(audit.Body, []byte(mediaID)) {
		t.Fatalf("删除成功应留媒体审计事件: %d %s", audit.StatusCode, audit.Body)
	}
	if dr := doJSON(t, mux, "DELETE", "/admin/api/media/"+mediaID, "", auth, csrf); dr.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删图应 404，得 %d", dr.StatusCode)
	}
	audit = doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, "")
	if n := bytes.Count(audit.Body, []byte(`"action":"media.delete"`)); n != 1 {
		t.Fatalf("失败重复删除不应追加审计事件，得 %d: %s", n, audit.Body)
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
	if audit := doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, ""); !bytes.Contains(audit.Body, []byte(`"action":"market.settings_update"`)) {
		t.Fatalf("保存市场设置应留审计事件: %s", audit.Body)
	}
}

func TestHTTPShippingSettingsSeparated(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	catalog := doJSON(t, mux, http.MethodGet, "/admin/api/settings/shipping/countries", "", auth, "")
	if catalog.StatusCode != http.StatusOK || !bytes.Contains(catalog.Body, []byte(`"continent":"Asia"`)) || !bytes.Contains(catalog.Body, []byte(`"code":"CN"`)) {
		t.Fatalf("配送国家目录异常: %d %s", catalog.StatusCode, catalog.Body)
	}
	if bad := doJSON(t, mux, http.MethodPut, "/admin/api/settings/shipping/countries", `{"countries":["XX"]}`, auth, csrf); bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法国家代码应拒绝，得 %d", bad.StatusCode)
	}
	if saved := doJSON(t, mux, http.MethodPut, "/admin/api/settings/shipping/countries", `{"countries":["CN","US"]}`, auth, csrf); saved.StatusCode != http.StatusOK {
		t.Fatalf("保存配送范围失败: %d %s", saved.StatusCode, saved.Body)
	}
	if saved := doJSON(t, mux, http.MethodPut, "/admin/api/settings/shipping/default", `{"rate_cents":500,"free_over_cents":20000}`, auth, csrf); saved.StatusCode != http.StatusOK {
		t.Fatalf("保存默认运费失败: %d %s", saved.StatusCode, saved.Body)
	}
	created := doJSON(t, mux, http.MethodPost, "/admin/api/settings/shipping", `{"name":"China","countries":"CN","rate_cents":900,"free_over_cents":0}`, auth, csrf)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("新增特殊规则失败: %d %s", created.StatusCode, created.Body)
	}
	var result struct {
		PublicID string `json:"public_id"`
	}
	if err := json.Unmarshal(created.Body, &result); err != nil || result.PublicID == "" {
		t.Fatalf("特殊规则响应异常: %s", created.Body)
	}
	duplicate := doJSON(t, mux, http.MethodPost, "/admin/api/settings/shipping", `{"name":"Duplicate","countries":"CN","rate_cents":1000,"free_over_cents":0}`, auth, csrf)
	if duplicate.StatusCode != http.StatusConflict {
		t.Fatalf("重复国家规则应冲突，得 %d %s", duplicate.StatusCode, duplicate.Body)
	}
	updated := doJSON(t, mux, http.MethodPatch, "/admin/api/settings/shipping/"+result.PublicID, `{"name":"China Updated","countries":"CN","rate_cents":800,"free_over_cents":10000}`, auth, csrf)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("更新特殊规则失败: %d %s", updated.StatusCode, updated.Body)
	}
	deleted := doJSON(t, mux, http.MethodDelete, "/admin/api/settings/shipping/"+result.PublicID, "", auth, csrf)
	if deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("删除特殊规则失败: %d %s", deleted.StatusCode, deleted.Body)
	}
	if audit := doJSON(t, mux, http.MethodGet, "/admin/api/audit-events", "", auth, ""); !bytes.Contains(audit.Body, []byte(`"action":"shipping.countries_update"`)) || !bytes.Contains(audit.Body, []byte(`"action":"shipping.default_update"`)) || !bytes.Contains(audit.Body, []byte(`"action":"shipping_zone.delete"`)) {
		t.Fatalf("配送设置审计事件不完整: %s", audit.Body)
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
	if audit := doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, ""); !bytes.Contains(audit.Body, []byte(`"action":"domain.settings_update"`)) {
		t.Fatalf("保存域名应留审计事件: %s", audit.Body)
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

// TestDashboard 播种订单/商品/库存后断言概览聚合：今日/近7日数与销售额、refunded 扣减、待处理、库存告警、开店进度。
func TestDashboard(t *testing.T) {
	h, mux := newHTTP(t)
	sess, _ := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}
	db := h.svc.db

	const layout = "2006-01-02T15:04:05.000Z"
	now := time.Now().UTC()
	tsNow := now.Format(layout)                         // 今日
	ts3d := now.Add(-72 * time.Hour).Format(layout)     // 近7日内、非今日
	ts8d := now.Add(-8 * 24 * time.Hour).Format(layout) // 近7日外

	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("播种失败 [%s]: %v", q, err)
		}
	}
	// 客户。
	exec(`INSERT INTO customer (id, public_id, email) VALUES (1, 'cust-1', 'a@b.co')`)
	// 订单：状态/金额/创建时间不同，验证今日/近7日窗口与 D6 销售额口径。
	ord := func(pub, status string, total int64, ts string) {
		exec(`INSERT INTO "order" (public_id, customer_id, status, email, ship_name, ship_address, currency, subtotal_cents, total_cents, created_at)
			VALUES (?, 1, ?, 'a@b.co', 'N', 'A', 'USD', ?, ?, ?)`, pub, status, total, total, ts)
	}
	ord("o1", "paid", 10000, tsNow)     // 今日已付
	ord("o2", "fulfilled", 5000, tsNow) // 今日已履约（计入销售额）
	ord("o3", "pending", 3000, tsNow)   // 今日未付（不计销售额）
	ord("o4", "refunded", 9999, tsNow)  // 今日已退款（D6：整单 refunded，全额不计）
	ord("o5", "paid", 2000, ts3d)       // 3天前已付（近7日、非今日）
	ord("o6", "paid", 7000, ts8d)       // 8天前已付（近7日外）

	// 商品：2 活 + 1 软删。
	exec(`INSERT INTO product (id, public_id, title, slug, status) VALUES (1,'p1','P1','p1','active')`)
	exec(`INSERT INTO product (id, public_id, title, slug, status) VALUES (2,'p2','P2','p2','active')`)
	exec(`INSERT INTO product (id, public_id, title, slug, status, deleted_at) VALUES (3,'p3','P3','p3','active', ?)`, tsNow)
	// 变体 + 库存：可售=quantity-reserved。
	vrt := func(id int64, pid int64, key string, qty, reserved int64) {
		exec(`INSERT INTO variant (id, public_id, product_id, option_key, price_cents) VALUES (?, ?, ?, ?, 100)`, id, "v"+key, pid, key)
		exec(`INSERT INTO inventory (variant_id, quantity, reserved) VALUES (?, ?, ?)`, id, qty, reserved)
	}
	vrt(1, 1, "a", 10, 10) // 可售 0 → 零库存
	vrt(2, 1, "b", 5, 2)   // 可售 3 → 低库存
	vrt(3, 2, "c", 20, 0)  // 可售 20 → 健康
	vrt(4, 3, "d", 0, 0)   // 可售 0 但商品软删 → 不计

	// 请求概览。
	r := doJSON(t, mux, "GET", "/admin/api/dashboard", "", auth, "")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("dashboard 应 200，得 %d %s", r.StatusCode, r.Body)
	}
	type window struct {
		Count      int64 `json:"count"`
		SalesCents int64 `json:"sales_cents"`
	}
	var dash struct {
		Currency string `json:"currency"`
		Orders   struct {
			Today              window `json:"today"`
			Week               window `json:"week"`
			PendingFulfillment int64  `json:"pending_fulfillment"`
		} `json:"orders"`
		Products struct {
			Count     int64 `json:"count"`
			ZeroStock int64 `json:"zero_stock"`
			LowStock  int64 `json:"low_stock"`
		} `json:"products"`
		Setup struct {
			HasProducts       bool `json:"has_products"`
			PaymentConfigured bool `json:"payment_configured"`
			DomainConfigured  bool `json:"domain_configured"`
			Ready             bool `json:"ready"`
		} `json:"setup"`
	}
	if err := json.Unmarshal(r.Body, &dash); err != nil {
		t.Fatalf("解析概览失败: %v (%s)", err, r.Body)
	}
	// 今日：4 单（o1-o4），销售额=15000（仅 paid+fulfilled，pending/refunded 不计）。
	if dash.Orders.Today.Count != 4 || dash.Orders.Today.SalesCents != 15000 {
		t.Fatalf("今日应 count=4 sales=15000，得 count=%d sales=%d", dash.Orders.Today.Count, dash.Orders.Today.SalesCents)
	}
	// 近7日：5 单（含 o5），销售额=17000（15000+2000）；8天前 o6 不计。
	if dash.Orders.Week.Count != 5 || dash.Orders.Week.SalesCents != 17000 {
		t.Fatalf("近7日应 count=5 sales=17000，得 count=%d sales=%d", dash.Orders.Week.Count, dash.Orders.Week.SalesCents)
	}
	// 待处理=status='paid' 全时=3（o1/o5/o6）。
	if dash.Orders.PendingFulfillment != 3 {
		t.Fatalf("待处理应=3，得 %d", dash.Orders.PendingFulfillment)
	}
	// 商品数=2（软删排除）；库存告警 零=1 低=1（软删商品变体不计）。
	if dash.Products.Count != 2 || dash.Products.ZeroStock != 1 || dash.Products.LowStock != 1 {
		t.Fatalf("商品应 count=2 zero=1 low=1，得 count=%d zero=%d low=%d", dash.Products.Count, dash.Products.ZeroStock, dash.Products.LowStock)
	}
	// 开店进度：有商品但未配收款/域名 → ready=false。
	if !dash.Setup.HasProducts || dash.Setup.PaymentConfigured || dash.Setup.DomainConfigured || dash.Setup.Ready {
		t.Fatalf("setup 应 has_products=true 其余 false，得 %+v", dash.Setup)
	}
	if dash.Currency != "USD" {
		t.Fatalf("货币应=USD（默认市场），得 %q", dash.Currency)
	}
}

// TestDashboardAuth 未登录访问概览应 401。
func TestDashboardAuth(t *testing.T) {
	_, mux := newHTTP(t)
	if r := doJSON(t, mux, "GET", "/admin/api/dashboard", "", nil, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，得 %d", r.StatusCode)
	}
}

func TestDiagnostics(t *testing.T) {
	_, mux := newHTTP(t)
	sess, _ := loginAndCookies(t, mux)
	r := doJSON(t, mux, "GET", "/admin/api/diagnostics", "", []*http.Cookie{sess}, "")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("diagnostics 应 200，得 %d %s", r.StatusCode, r.Body)
	}
	if !bytes.Contains(r.Body, []byte(`"database":{"status":"ok"}`)) || !bytes.Contains(r.Body, []byte(`"asset_count":0`)) || !bytes.Contains(r.Body, []byte(`"automatic_count":0`)) {
		t.Fatalf("diagnostics 响应异常: %s", r.Body)
	}
}

func TestBackupSettings(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}
	if r := doJSON(t, mux, "GET", "/admin/api/settings/backup", "", auth, ""); r.StatusCode != http.StatusOK || !bytes.Contains(r.Body, []byte(`"interval":"24h0m0s"`)) {
		t.Fatalf("初始备份设置异常: %d %s", r.StatusCode, r.Body)
	}
	r := doJSON(t, mux, "PUT", "/admin/api/settings/backup", `{"interval":"90m","retention":"12"}`, auth, csrf)
	if r.StatusCode != http.StatusOK || !bytes.Contains(r.Body, []byte(`"interval":"90m"`)) || !bytes.Contains(r.Body, []byte(`"retention":12`)) {
		t.Fatalf("保存备份设置失败: %d %s", r.StatusCode, r.Body)
	}
	if r := doJSON(t, mux, "PUT", "/admin/api/settings/backup", `{"interval":"30s","retention":"0"}`, auth, csrf); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法设置应 400: %d %s", r.StatusCode, r.Body)
	}
}

func TestBackupRemoteTestNotConfigured(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	r := doJSON(t, mux, http.MethodPost, "/admin/api/settings/backup/test", "", auth, csrf)
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未启用 WebDAV 时远端测试应 400，得 %d %s", r.StatusCode, r.Body)
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

// ---- SMTP 设置 + 向导邮件步骤（M4.3）----

// TestSMTPSettingsAndWizard 覆盖：校验拒非法、valid 存 + password 密文、configured/needed 联动、skip、CSRF。
func TestSMTPSettingsAndWizard(t *testing.T) {
	h, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	// 初始未配置未跳过 → needed=true。
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/smtp", "", auth, ""); !bytes.Contains(r.Body, []byte(`"needed":true`)) {
		t.Fatalf("初始应 needed=true: %s", r.Body)
	}

	// 非法输入 → 400：缺 host、端口非数字、加密非法、发件地址非邮箱。
	for _, body := range []string{
		`{"host":"","port":"587","from_address":"a@b.co","encryption":"starttls"}`,
		`{"host":"smtp.x.com","port":"abc","from_address":"a@b.co","encryption":"starttls"}`,
		`{"host":"smtp.x.com","port":"587","from_address":"a@b.co","encryption":"weird"}`,
		`{"host":"smtp.x.com","port":"587","from_address":"notmail","encryption":"starttls"}`,
	} {
		if r := doJSON(t, mux, "PUT", "/admin/api/settings/smtp", body, auth, csrf); r.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法 SMTP %s 应 400，得 %d %s", body, r.StatusCode, r.Body)
		}
	}

	// 缺 CSRF → 403。
	if r := doJSON(t, mux, "PUT", "/admin/api/settings/smtp", `{"host":"smtp.x.com","port":"587","from_address":"a@b.co","encryption":"starttls"}`, auth, ""); r.StatusCode != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应 403，得 %d", r.StatusCode)
	}

	// 合法保存（含密码）→ 200、configured=true、has_password=true。
	const pw = "smtp-secret-xyz"
	ok := doJSON(t, mux, "PUT", "/admin/api/settings/smtp",
		`{"host":"smtp.example.com","port":"587","username":"u@example.com","password":"`+pw+`","from_address":"shop@example.com","from_name":"Shop","encryption":"starttls"}`, auth, csrf)
	if ok.StatusCode != http.StatusOK || !bytes.Contains(ok.Body, []byte(`"configured":true`)) || !bytes.Contains(ok.Body, []byte(`"has_password":true`)) {
		t.Fatalf("合法保存应 200 configured/has_password: %d %s", ok.StatusCode, ok.Body)
	}

	// 密码落库应为密文（encrypted=1 且不含明文）。
	var val string
	var enc int
	if err := h.svc.db.QueryRow(`SELECT value, encrypted FROM setting WHERE key='smtp.password'`).Scan(&val, &enc); err != nil {
		t.Fatalf("读 smtp.password 失败: %v", err)
	}
	if enc != 1 || bytes.Contains([]byte(val), []byte(pw)) {
		t.Fatalf("密码应加密存储：encrypted=%d 含明文=%v", enc, bytes.Contains([]byte(val), []byte(pw)))
	}
	if audit := doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, ""); !bytes.Contains(audit.Body, []byte(`"action":"smtp.settings_update"`)) || bytes.Contains(audit.Body, []byte(pw)) {
		t.Fatalf("保存 SMTP 应留审计且绝不泄露密码: %s", audit.Body)
	}

	// 配置后 → 向导 needed=false。
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/smtp", "", auth, ""); !bytes.Contains(r.Body, []byte(`"needed":false`)) {
		t.Fatalf("配好后应 needed=false: %s", r.Body)
	}
}

// TestWizardSMTPSkip 覆盖跳过：skip→needed=false→skip 缺 CSRF→403。
func TestWizardSMTPSkip(t *testing.T) {
	_, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{sess}

	if r := doJSON(t, mux, "POST", "/admin/api/wizard/smtp/skip", "", auth, csrf); r.StatusCode != http.StatusOK {
		t.Fatalf("skip 应 200: %d %s", r.StatusCode, r.Body)
	}
	if r := doJSON(t, mux, "GET", "/admin/api/wizard/smtp", "", auth, ""); !bytes.Contains(r.Body, []byte(`"needed":false`)) {
		t.Fatalf("跳过后应 needed=false: %s", r.Body)
	}
	if r := doJSON(t, mux, "POST", "/admin/api/wizard/smtp/skip", "", auth, ""); r.StatusCode != http.StatusForbidden {
		t.Fatalf("skip 缺 CSRF 应 403，得 %d", r.StatusCode)
	}
}

// TestCookieSecureFollowsRequestTLS 锁死 D8-A：cookie 的 Secure 标记按**请求实际是否走 TLS**决定，
// 而非静态的 Env=="prod"。session 与 csrf 两条各测两分支。
//
// 背景（真机实证）：prod 明文态下浏览器会整条丢弃明文来源的 `Set-Cookie; Secure`
// （RFC 6265bis §5.5），表现为 login 返 200 而随后 me 返 401，商家永远登不进后台。
// 若只修 session 漏了 csrf，则变成「能登进去但所有写操作 403」，更隐蔽。
func TestCookieSecureFollowsRequestTLS(t *testing.T) {
	_, mux := newHTTP(t)
	doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")

	// login 一次，返回该请求下发的 session/csrf 两条 cookie 的 Secure 值。
	login := func(tls bool) map[string]bool {
		t.Helper()
		req := httptest.NewRequest("POST", "/admin/api/login",
			bytes.NewBufferString(`{"username":"admin","password":"supersecret"}`))
		req.Header.Set("Content-Type", "application/json")
		if tls {
			req.TLS = &cryptotls.ConnectionState{} // httptest 默认 r.TLS==nil，显式置非 nil 模拟 TLS 请求
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		res := rec.Result()
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("登录应 200，得 %d", res.StatusCode)
		}
		got := map[string]bool{}
		for _, c := range res.Cookies() {
			got[c.Name] = c.Secure
		}
		return got
	}

	// ① TLS 请求 → 两条都必须带 Secure。
	overTLS := login(true)
	for _, name := range []string{sessionCookie, csrfCookie} {
		if secure, ok := overTLS[name]; !ok {
			t.Fatalf("TLS 请求应下发 %s", name)
		} else if !secure {
			t.Fatalf("TLS 请求下 %s 必须带 Secure", name)
		}
	}

	// ② 明文请求 → 两条都不得带 Secure（否则浏览器整条丢弃，商家登不进后台）。
	overPlain := login(false)
	for _, name := range []string{sessionCookie, csrfCookie} {
		if secure, ok := overPlain[name]; !ok {
			t.Fatalf("明文请求应下发 %s", name)
		} else if secure {
			t.Fatalf("明文请求下 %s 绝不能带 Secure（会被浏览器丢弃）", name)
		}
	}

	// ③ 其余 cookie 属性不受影响：session 必须 HttpOnly、csrf 必须非 HttpOnly（供 SPA 读回传）、
	//    两者均 SameSite=Lax。
	req := httptest.NewRequest("POST", "/admin/api/login",
		bytes.NewBufferString(`{"username":"admin","password":"supersecret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	for _, c := range res.Cookies() {
		if c.SameSite != http.SameSiteLaxMode {
			t.Fatalf("%s 的 SameSite 应保持 Lax，得 %v", c.Name, c.SameSite)
		}
		switch c.Name {
		case sessionCookie:
			if !c.HttpOnly {
				t.Fatal("session cookie 必须保持 HttpOnly")
			}
		case csrfCookie:
			if c.HttpOnly {
				t.Fatal("csrf cookie 必须保持非 HttpOnly（SPA 需读取回传）")
			}
		}
	}
}

// TestCookieSecureOnLogout 登出清 cookie 时同样按请求 TLS 决定 Secure——
// 机理：Secure 不属于 cookie 的身份三元组(name/domain/path)，删除并不靠它匹配；
// 而是明文来源发出的带 Secure 的 Set-Cookie 会被整条丢弃，那条 Max-Age=-1
// 的清除指令根本没被处理 → 登出静默失败、cookie 仍在。
func TestCookieSecureOnLogout(t *testing.T) {
	_, mux := newHTTP(t)
	doJSON(t, mux, "POST", "/admin/api/setup", `{"username":"admin","password":"supersecret"}`, nil, "")

	for _, tc := range []struct {
		name      string
		tls, want bool
	}{
		{"明文登出 → 不带 Secure", false, false},
		{"TLS 登出 → 带 Secure", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 每个子用例各自登录一次：登出会作废会话，复用同一会话会让第二个子用例拿不到清除指令。
			lr := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"admin","password":"supersecret"}`, nil, "")
			var csrf string
			for _, c := range lr.Cookies {
				if c.Name == csrfCookie {
					csrf = c.Value
				}
			}
			req := httptest.NewRequest("POST", "/admin/api/logout", nil)
			req.Header.Set(csrfHeader, csrf)
			for _, c := range lr.Cookies {
				req.AddCookie(c)
			}
			if tc.tls {
				req.TLS = &cryptotls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			res := rec.Result()
			defer func() { _ = res.Body.Close() }()
			n := 0
			for _, c := range res.Cookies() {
				if c.MaxAge < 0 {
					n++
					if c.Secure != tc.want {
						t.Fatalf("%s 清除指令 Secure=%v，期望 %v", c.Name, c.Secure, tc.want)
					}
				}
			}
			if n != 2 {
				t.Fatalf("登出应清 session+csrf 两条，得 %d 条", n)
			}
		})
	}
}
