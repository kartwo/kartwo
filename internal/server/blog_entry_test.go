// 博客媒体贯通测试 / Blog Media Entry Integration Tests
// 功能：真实后台上传/发布、店面图片与安全渲染、旧文章转换、员工和演示边界的隔离回归
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 10:12:26
package server

import (
	"bytes"
	"encoding/json"
	"github.com/kartwo/kartwo/internal/blog"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBlogImagePublicationLegacyAndPermissionFlow(t *testing.T) {
	handler, st, root := newAdminEntryFixture(t)
	call := func(method, path string, body []byte, contentType string, cookies []*http.Cookie, csrf string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "https://blog.example.invalid"+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("Origin", "https://blog.example.invalid")
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s status%d want%d", method, path, w.Code, want)
		}
		if strings.HasPrefix(path, "/admin/api/") {
			// Invitation responses use the equivalent no-store directive
			// without the redundant private directive. Check cache semantics, not spelling.
			noStore := false
			for _, directive := range strings.Split(w.Header().Get("Cache-Control"), ",") {
				noStore = noStore || strings.EqualFold(strings.TrimSpace(directive), "no-store")
			}
			if !noStore || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Robots-Tag") != "noindex, nofollow, noarchive" {
				t.Fatal("private API lost response protection")
			}
		}
		if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("actual server security middleware missing")
		}
		return w
	}
	jsonCall := func(method, path string, payload any, cookies []*http.Cookie, csrf string, want int) *httptest.ResponseRecorder {
		t.Helper()
		data, e := json.Marshal(payload)
		if e != nil {
			t.Fatal(e)
		}
		return call(method, path, data, "application/json", cookies, csrf, want)
	}
	decode := func(w *httptest.ResponseRecorder, value any) {
		t.Helper()
		if e := json.Unmarshal(w.Body.Bytes(), value); e != nil {
			t.Fatal("invalid response contract", e)
		}
	}
	authCookies := func(w *httptest.ResponseRecorder) ([]*http.Cookie, string) {
		t.Helper()
		var csrf string
		var session *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "kartwo_session" {
				session = c
			}
			if c.Name == "kartwo_csrf" {
				csrf = c.Value
			}
		}
		if session == nil || csrf == "" {
			t.Fatal("normal login did not issue session and CSRF")
		}
		return []*http.Cookie{session}, csrf
	}
	jsonCall("POST", "/admin/api/setup", map[string]string{"username": "blog-fixture-owner", "password": "isolated-blog-owner-password"}, nil, "", 201)
	owner, csrf := authCookies(jsonCall("POST", "/admin/api/login", map[string]string{"username": "blog-fixture-owner", "password": "isolated-blog-owner-password"}, nil, "", 200))
	pngFixture := func(shift uint8) []byte {
		t.Helper()
		img := image.NewRGBA(image.Rect(0, 0, 80, 40))
		for y := 0; y < 40; y++ {
			for x := 0; x < 80; x++ {
				img.SetRGBA(x, y, color.RGBA{uint8(x) + shift, uint8(y) * 3, 150 - shift, 255})
			}
		}
		var data bytes.Buffer
		if e := png.Encode(&data, img); e != nil {
			t.Fatal(e)
		}
		return data.Bytes()
	}
	upload := func(data []byte, cookies []*http.Cookie, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, e := writer.CreateFormFile("file", "fixture.png")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = part.Write(data); e != nil {
			t.Fatal(e)
		}
		if e = writer.Close(); e != nil {
			t.Fatal(e)
		}
		return call("POST", "/admin/api/blog/images", body.Bytes(), writer.FormDataContentType(), cookies, token, want)
	}
	pngCover, pngBody := pngFixture(20), pngFixture(60)
	upload(pngCover, nil, "", 401)
	upload(pngCover, owner, "", 403)
	upload([]byte("<svg onload='unsafe'>not a PNG</svg>"), owner, csrf, 415)
	var cover, body, repeated struct {
		Path   string `json:"path"`
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	decode(upload(pngCover, owner, csrf, 201), &cover)
	decode(upload(pngBody, owner, csrf, 201), &body)
	decode(upload(pngCover, owner, csrf, 201), &repeated)
	imagePath := regexp.MustCompile(`^blog/[a-f0-9]{64}\.webp$`)
	for _, image := range []struct {
		Path   string `json:"path"`
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}{cover, body, repeated} {
		if !imagePath.MatchString(image.Path) || image.URL != "/media/"+image.Path || image.Width != 80 || image.Height != 40 {
			t.Fatal("frontend image contract changed")
		}
		stored, e := os.ReadFile(filepath.Join(root, image.Path))
		if e != nil || len(stored) < 12 || string(stored[:4]) != "RIFF" || string(stored[8:12]) != "WEBP" {
			t.Fatal("stored blog asset is not WebP", e)
		}
		served := call("GET", image.URL, nil, "", nil, "", 200)
		if served.Header().Get("Content-Type") != "image/webp" || !bytes.Equal(served.Body.Bytes(), stored) {
			t.Fatal("storefront media URL does not serve actual converted bytes")
		}
	}
	if repeated.Path != cover.Path || body.Path == cover.Path {
		t.Fatal("hash deduplication or distinct content failed")
	}
	assets, err := filepath.Glob(filepath.Join(root, "blog", "*.webp"))
	if err != nil || len(assets) != 2 {
		t.Fatal("repeat upload duplicated stored asset", err)
	}
	html := `<h2>Running checklist</h2><p><strong>Choose comfortable layers.</strong></p><table><tbody><tr><td>Size</td><td>Fit</td></tr></tbody></table><p><img src="` + body.URL + `" alt="Virtual running media" onerror="UNSAFE-BLOG-MARKER"></p><iframe src="https://www.youtube-nocookie.com/embed/KARTWO_TEST"></iframe><script>UNSAFE-BLOG-MARKER</script>`
	post := map[string]any{"title": "Integrated Running Guide", "slug": "integrated-running-guide", "excerpt": "An isolated running article.", "seo_description": "Isolated SEO fixture", "body_html": html, "body_markdown": "", "cover_path": cover.Path, "status": "draft"}
	var created struct {
		ID string `json:"public_id"`
	}
	decode(jsonCall("POST", "/admin/api/blog/posts", post, owner, csrf, 201), &created)
	path := "/admin/api/blog/posts/" + created.ID
	var saved blog.Post
	decode(jsonCall("GET", path, nil, owner, "", 200), &saved)
	for _, fragment := range []string{"<h2>", "<strong>", "<table>", body.URL, `referrerpolicy="strict-origin"`, `https://www.youtube-nocookie.com/embed/KARTWO_TEST`} {
		if !strings.Contains(saved.BodyHTML, fragment) {
			t.Fatalf("safe rich content lost %s", fragment)
		}
	}
	if strings.Contains(saved.BodyHTML, "onerror") || strings.Contains(saved.BodyHTML, "<script") || strings.Contains(saved.BodyHTML, "UNSAFE-BLOG-MARKER") {
		t.Fatal("unsafe submitted content survived sanitization")
	}
	call("GET", "/blog/integrated-running-guide", nil, "", nil, "", 404)
	post["slug"] = "changed-link"
	jsonCall("PUT", path, post, owner, csrf, 400)
	post["slug"] = "integrated-running-guide"
	post["status"] = "active"
	jsonCall("PUT", path, post, owner, csrf, 200)
	page := call("GET", "/blog/integrated-running-guide", nil, "", nil, "", 200)
	list := call("GET", "/blog", nil, "", nil, "", 200)
	for _, fragment := range []string{"Integrated Running Guide", body.URL, "<h2>", "<table>", `referrerpolicy="strict-origin"`, `https://www.youtube-nocookie.com/embed/KARTWO_TEST`} {
		if !strings.Contains(page.Body.String(), fragment) {
			t.Fatalf("published article missing %s", fragment)
		}
	}
	if !strings.Contains(list.Body.String(), cover.URL) || strings.Contains(page.Body.String(), "UNSAFE-BLOG-MARKER") {
		t.Fatal("list cover or storefront escaping failed")
	}
	var role struct {
		ID string `json:"public_id"`
	}
	decode(jsonCall("POST", "/admin/api/roles", map[string]any{"name": "Blog fixture reader", "permissions": []string{"blog.view"}}, owner, csrf, 201), &role)
	var invite struct {
		Path string `json:"invite_path"`
	}
	decode(jsonCall("POST", "/admin/api/users", map[string]string{"username": "blog-fixture-reader", "role_id": role.ID}, owner, csrf, 201), &invite)
	link, e := url.Parse(invite.Path)
	if e != nil {
		t.Fatal(e)
	}
	fragment, e := url.Parse(link.Fragment)
	if e != nil {
		t.Fatal(e)
	}
	jsonCall("POST", "/admin/api/invites/accept", map[string]string{"token": fragment.Query().Get("token"), "password": "isolated-blog-reader-password"}, nil, "", 200)
	reader, readerCSRF := authCookies(jsonCall("POST", "/admin/api/login", map[string]string{"username": "blog-fixture-reader", "password": "isolated-blog-reader-password"}, nil, "", 200))
	jsonCall("GET", path, nil, reader, "", 200)
	jsonCall("PUT", path, post, reader, readerCSRF, 403)
	upload(pngBody, reader, readerCSRF, 403)
	demo, demoCSRF := authCookies(jsonCall("POST", "/admin/api/demo-session", nil, nil, "", 201))
	var demoPosts struct {
		Posts []blog.Post `json:"posts"`
	}
	decode(jsonCall("GET", "/admin/api/blog/posts", nil, demo, "", 200), &demoPosts)
	if len(demoPosts.Posts) != 0 {
		t.Fatal("demo exposed real article content")
	}
	jsonCall("GET", path, nil, demo, "", 404)
	jsonCall("PUT", path, post, demo, demoCSRF, 403)
	upload(pngBody, demo, demoCSRF, 403)
	legacyMarkdown := "## Legacy heading\n\nOriginal Markdown body.\n\n![Legacy image](" + body.URL + ")"
	legacy := map[string]any{"title": "Legacy Running Notes", "slug": "legacy-running-notes", "body_markdown": legacyMarkdown, "status": "draft"}
	var legacyID struct {
		ID string `json:"public_id"`
	}
	decode(jsonCall("POST", "/admin/api/blog/posts", legacy, owner, csrf, 201), &legacyID)
	var preview struct {
		HTML string `json:"html"`
	}
	decode(jsonCall("POST", "/admin/api/blog/preview", map[string]string{"body_markdown": legacyMarkdown}, owner, csrf, 200), &preview)
	if !strings.Contains(preview.HTML, body.URL) {
		t.Fatal("legacy conversion dropped existing local image")
	}
	legacy["body_html"], legacy["body_markdown"] = preview.HTML, ""
	jsonCall("PUT", "/admin/api/blog/posts/"+legacyID.ID, legacy, owner, csrf, 200)
	decode(jsonCall("GET", "/admin/api/blog/posts/"+legacyID.ID, nil, owner, "", 200), &saved)
	if saved.BodyMarkdown != "" || !strings.Contains(saved.BodyHTML, "Legacy heading") || !strings.Contains(saved.BodyHTML, body.URL) {
		t.Fatal("converted legacy article did not persist")
	}
	jsonCall("PUT", "/admin/api/settings/blog", map[string]bool{"enabled": false}, owner, csrf, 200)
	call("GET", "/blog", nil, "", nil, "", 404)
	call("GET", "/blog/integrated-running-guide", nil, "", nil, "", 404)
	jsonCall("GET", path, nil, owner, "", 200)
	jsonCall("PUT", "/admin/api/settings/blog", map[string]bool{"enabled": true}, owner, csrf, 200)
	call("GET", "/blog/integrated-running-guide", nil, "", nil, "", 200)
	var posts, uploads, mailRows, refunds int
	if e = st.DB.QueryRow(`SELECT (SELECT count(*) FROM blog_post),(SELECT count(*) FROM audit_event WHERE action='blog.image_upload'),(SELECT count(*) FROM email_outbox),(SELECT count(*) FROM refund_request)`).Scan(&posts, &uploads, &mailRows, &refunds); e != nil || posts != 2 || uploads != 3 || mailRows != 0 || refunds != 0 {
		t.Fatal("permission denial duplicated media/business side effects", e)
	}
}
