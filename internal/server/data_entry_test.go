// 导入导出恢复贯通测试 / Import Export Restore Entry Tests
// 功能：正式鉴权/CSV幂等/媒体ZIP/私密排除与恢复后原任务重放的隔离验证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 12:26:11
package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/importer"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/store"
)

type dataEntryClient struct {
	t       *testing.T
	handler http.Handler
	cookies []*http.Cookie
	csrf    string
}

func (c dataEntryClient) send(method, path string, data []byte, contentType string, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	r := httptest.NewRequest(method, "https://entry.example.invalid"+path, bytes.NewReader(data))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Origin", "https://entry.example.invalid")
	r.Header.Set("X-CSRF-Token", c.csrf)
	for _, cookie := range c.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if w.Code != status {
		c.t.Fatalf("%s %s status%d want%d", method, path, w.Code, status)
	}
	if strings.HasPrefix(path, "/admin/api/") && (!strings.Contains(w.Header().Get("Cache-Control"), "no-store") || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Robots-Tag") != "noindex, nofollow, noarchive") {
		c.t.Fatal("private data entry response lost privacy headers")
	}
	if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		c.t.Fatal("actual server security middleware missing")
	}
	return w
}
func (c dataEntryClient) json(method, path string, payload any, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	b, e := json.Marshal(payload)
	if e != nil {
		c.t.Fatal(e)
	}
	return c.send(method, path, b, "application/json", status)
}
func (c dataEntryClient) decode(w *httptest.ResponseRecorder, value any) {
	c.t.Helper()
	if e := json.Unmarshal(w.Body.Bytes(), value); e != nil {
		c.t.Fatal("invalid entry response", e)
	}
}
func (c dataEntryClient) signed(w *httptest.ResponseRecorder) dataEntryClient {
	c.t.Helper()
	c.cookies = w.Result().Cookies()
	c.csrf = ""
	for _, cookie := range c.cookies {
		if cookie.Name == "kartwo_csrf" {
			c.csrf = cookie.Value
		}
	}
	if c.csrf == "" {
		c.t.Fatal("normal authentication did not issue CSRF")
	}
	return c
}
func (c dataEntryClient) multipart(path, name, content string, extra map[string]string, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, e := writer.CreateFormFile("file", name)
	if e != nil {
		c.t.Fatal(e)
	}
	if _, e = file.Write([]byte(content)); e != nil {
		c.t.Fatal(e)
	}
	for key, value := range extra {
		if e = writer.WriteField(key, value); e != nil {
			c.t.Fatal(e)
		}
	}
	if e = writer.Close(); e != nil {
		c.t.Fatal(e)
	}
	return c.send("POST", path, body.Bytes(), writer.FormDataContentType(), status)
}
func dataEntryToken(t *testing.T) string {
	t.Helper()
	v, e := order.NewGuestToken()
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func dataEntryOwner(t *testing.T, handler http.Handler) (dataEntryClient, string) {
	t.Helper()
	client := dataEntryClient{t: t, handler: handler}
	password := dataEntryToken(t)
	client.json("POST", "/admin/api/setup", map[string]string{"username": "entry-owner", "password": password}, 201)
	return client.signed(client.json("POST", "/admin/api/login", map[string]string{"username": "entry-owner", "password": password}, 200)), password
}

const dataEntryGenericCSV = "title,slug,status,description,option1_name,option1_value,option2_name,option2_value,sku,price_cents,quantity\nEntry Running Tee,entry-running-tee,active,Light running layer,Size,S,,,ENTRY-S,1299,4\nEntry Running Tee,entry-running-tee,active,Light running layer,Size,M,,,ENTRY-M,1599,6\n"
const dataEntryShopifyCSV = "Handle,Title,Body (HTML),Option1 Name,Option1 Value,Option2 Name,Option2 Value,Option3 Name,Option3 Value,Variant SKU,Variant Inventory Qty,Variant Price,Status,Image Src\nentry-running-cap,Entry Running Cap,<p>Lightweight</p>,Size,One,,,,,ENTRY-CAP,10,19.99,active,\n"

func dataEntryCounts(t *testing.T, st *store.Store, wantProducts, wantVariants, wantJobs int) {
	t.Helper()
	var products, variants, jobs, orders, emails, refunds int
	e := st.DB.QueryRow(`SELECT (SELECT count(*) FROM product),(SELECT count(*) FROM variant),(SELECT count(*) FROM import_job),(SELECT count(*) FROM "order"),(SELECT count(*) FROM email_outbox),(SELECT count(*) FROM refund_request)`).Scan(&products, &variants, &jobs, &orders, &emails, &refunds)
	if e != nil || products != wantProducts || variants != wantVariants || jobs != wantJobs || orders != 0 || emails != 0 || refunds != 0 {
		t.Fatalf("unexpected isolated facts products%d variants%d jobs%d orders%d email%d refunds%d err%v", products, variants, jobs, orders, emails, refunds, e)
	}
}
func dataEntryVariants(t *testing.T, st *store.Store) {
	t.Helper()
	rows, err := st.DB.Query(`SELECT v.sku,v.price_cents,i.quantity,i.reserved FROM variant v JOIN inventory i ON i.variant_id=v.id ORDER BY v.sku`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	want := map[string][3]int64{"ENTRY-S": {1299, 4, 0}, "ENTRY-M": {1599, 6, 0}, "ENTRY-CAP": {1999, 10, 0}}
	count := 0
	for rows.Next() {
		var sku string
		var facts [3]int64
		if err = rows.Scan(&sku, &facts[0], &facts[1], &facts[2]); err != nil {
			t.Fatal(err)
		}
		expected, ok := want[sku]
		if !ok || facts != expected {
			t.Fatal("imported price/stock changed")
		}
		delete(want, sku)
		count++
	}
	if err = rows.Err(); err != nil || count != 3 || len(want) != 0 {
		t.Fatal("imported variant facts incomplete", err)
	}
}

func TestDataImportExportRestoreFlow(t *testing.T) {
	handler, st, mediaRoot := newAdminEntryFixture(t)
	owner, password := dataEntryOwner(t, handler)
	anonymous := dataEntryClient{t: t, handler: handler}
	var role struct {
		ID string `json:"public_id"`
	}
	owner.decode(owner.json("POST", "/admin/api/roles", map[string]any{"name": "Entry importer", "permissions": []string{"imports.manage"}}, 201), &role)
	var invite struct {
		Path string `json:"invite_path"`
	}
	owner.decode(owner.json("POST", "/admin/api/users", map[string]string{"username": "entry-importer", "role_id": role.ID}, 201), &invite)
	link, e := url.Parse(invite.Path)
	if e != nil {
		t.Fatal(e)
	}
	fragment, e := url.Parse(link.Fragment)
	if e != nil {
		t.Fatal(e)
	}
	staffPassword := dataEntryToken(t)
	anonymous.json("POST", "/admin/api/invites/accept", map[string]string{"token": fragment.Query().Get("token"), "password": staffPassword}, 200)
	staff := anonymous.signed(anonymous.json("POST", "/admin/api/login", map[string]string{"username": "entry-importer", "password": staffPassword}, 200))
	noCSRF := owner
	noCSRF.csrf = ""
	noCSRF.multipart("/admin/api/imports/csv/preview", "products.csv", dataEntryGenericCSV, map[string]string{"format": "generic"}, 403)
	anonymous.multipart("/admin/api/imports/csv/preview", "products.csv", dataEntryGenericCSV, map[string]string{"format": "generic"}, 401)
	dataEntryCounts(t, st, 0, 0, 0)
	var generic importer.Preview
	staff.decode(staff.multipart("/admin/api/imports/csv/preview", "products.csv", dataEntryGenericCSV, map[string]string{"format": "generic"}, 200), &generic)
	if generic.Status != "previewed" || generic.Format != "generic" || generic.ProductCount != 1 || generic.VariantCount != 2 || len(generic.Errors) != 0 {
		t.Fatal("generic preview contract")
	}
	dataEntryCounts(t, st, 0, 0, 1)
	jobPath := "/admin/api/imports/" + generic.PublicID
	noCSRF.json("POST", jobPath+"/execute", nil, 403)
	for range 2 {
		var result importer.Preview
		staff.decode(staff.json("POST", jobPath+"/execute", nil, 200), &result)
		if result.PublicID != generic.PublicID || result.Status != "succeeded" {
			t.Fatal("same job did not succeed idempotently")
		}
	}
	dataEntryCounts(t, st, 1, 2, 1)
	var repeated importer.Preview
	owner.decode(owner.multipart("/admin/api/imports/csv/preview", "again.csv", dataEntryGenericCSV, map[string]string{"format": "generic"}, 200), &repeated)
	if repeated.PublicID != generic.PublicID || repeated.Status != "succeeded" {
		t.Fatal("identical file did not recover original successful job")
	}
	metadata := staff.json("GET", jobPath, nil, 200).Body.String()
	if strings.Contains(metadata, "source_csv") || strings.Contains(metadata, "ENTRY-S") {
		t.Fatal("job metadata exposed stored CSV source")
	}
	staff.json("GET", "/admin/api/products", nil, 403)
	for _, slug := range []string{"entry-running-tee"} {
		page := anonymous.send("GET", "/p/"+slug, nil, "", 200)
		if !strings.Contains(page.Body.String(), "Entry Running Tee") {
			t.Fatal("imported product not visible")
		}
	}
	var bad importer.Preview
	owner.decode(owner.multipart("/admin/api/imports/csv/preview", "invalid.csv", strings.ReplaceAll(dataEntryGenericCSV, "1299", "1.5"), map[string]string{"format": "generic"}, 200), &bad)
	if bad.Status != "rejected" || len(bad.Errors) == 0 {
		t.Fatal("invalid integer price accepted")
	}
	owner.json("POST", "/admin/api/imports/"+bad.PublicID+"/execute", nil, 400)
	var shopify importer.Preview
	owner.decode(owner.multipart("/admin/api/imports/csv/preview", "shopify.csv", dataEntryShopifyCSV, map[string]string{"format": "shopify"}, 200), &shopify)
	if shopify.Format != "shopify" || shopify.Status != "previewed" || shopify.ProductCount != 1 || shopify.VariantCount != 1 {
		t.Fatal("Shopify preview contract")
	}
	owner.json("POST", "/admin/api/imports/"+shopify.PublicID+"/execute", nil, 200)
	redirect := anonymous.send("GET", "/products/entry-running-cap", nil, "", 301)
	if redirect.Header().Get("Location") != "/p/entry-running-cap" {
		t.Fatal("Shopify old URL not redirected")
	}
	dataEntryCounts(t, st, 2, 3, 3)
	dataEntryVariants(t, st)
	var productID string
	if e = st.DB.QueryRow(`SELECT public_id FROM product WHERE slug=?`, "entry-running-tee").Scan(&productID); e != nil {
		t.Fatal(e)
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 3), uint8(y * 5), 180, 255})
		}
	}
	var pngBytes bytes.Buffer
	if e = png.Encode(&pngBytes, img); e != nil {
		t.Fatal(e)
	}
	owner.multipart("/admin/api/products/"+productID+"/media", "fixture.png", pngBytes.String(), nil, 201)
	mediaFiles := map[string][32]byte{}
	e = filepath.WalkDir(mediaRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(mediaRoot, path)
		if e != nil {
			return e
		}
		mediaFiles["media/"+filepath.ToSlash(rel)] = sha256.Sum256(data)
		return nil
	})
	if e != nil || len(mediaFiles) == 0 {
		t.Fatal("real media fixture missing", e)
	}
	dataDir := filepath.Dir(mediaRoot)
	for _, name := range []string{"certs/cache", "logs/private.log", "backups/manual-marker", "private.env"} {
		path := filepath.Join(dataDir, name)
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, []byte("VIRTUAL-NONEXPORT-MARKER"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	secret := dataEntryToken(t)
	owner.json("PUT", "/admin/api/settings/smtp", map[string]string{"host": "smtp.entry.example.invalid", "port": "587", "username": "fixture", "password": secret, "from_address": "entry@example.invalid", "encryption": "starttls"}, 200)
	anonymous.json("GET", "/admin/api/export", nil, 401)
	staff.json("GET", "/admin/api/export", nil, 403)
	demo := anonymous.signed(anonymous.json("POST", "/admin/api/demo-session", nil, 201))
	demo.json("GET", "/admin/api/export", nil, 403)
	demo.json("GET", jobPath, nil, 403)
	demo.multipart("/admin/api/imports/csv/preview", "demo.csv", dataEntryGenericCSV, map[string]string{"format": "generic"}, 403)
	demo.json("POST", jobPath+"/execute", nil, 403)
	dataEntryCounts(t, st, 2, 3, 3)
	exported := owner.json("GET", "/admin/api/export", nil, 200)
	if exported.Header().Get("Content-Type") != "application/zip" || !strings.HasPrefix(exported.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("native download contract missing")
	}
	zipReader, e := zip.NewReader(bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
	if e != nil {
		t.Fatal(e)
	}
	archived := map[string][]byte{}
	for _, entry := range zipReader.File {
		if entry.Name != "manifest.json" && entry.Name != "shop.db" && !strings.HasPrefix(entry.Name, "media/") {
			t.Fatal("nonportable private path exported")
		}
		f, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(f)
		closeErr := f.Close()
		if e != nil || closeErr != nil {
			t.Fatal("archive entry read failed", e, closeErr)
		}
		archived[entry.Name] = data
	}
	var manifest backup.Manifest
	if e = json.Unmarshal(archived["manifest.json"], &manifest); e != nil || manifest.FormatVersion != 1 || manifest.AppVersion != "entry-flow-test" {
		t.Fatal("export manifest contract", e)
	}
	if len(archived) != len(mediaFiles)+2 || bytes.Contains(archived["shop.db"], []byte(secret)) {
		t.Fatal("incomplete media or plaintext secret in snapshot")
	}
	for name, hash := range mediaFiles {
		if sha256.Sum256(archived[name]) != hash {
			t.Fatal("exported media changed")
		}
	}
	if leftovers, e := filepath.Glob(filepath.Join(dataDir, "backups", ".export-*")); e != nil || len(leftovers) != 0 {
		t.Fatal("one-time export leaked temporary files", e)
	}
	archive := filepath.Join(t.TempDir(), "entry.zip")
	if e = os.WriteFile(archive, exported.Body.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	archiveHash := sha256.Sum256(exported.Body.Bytes())
	restoredDir := filepath.Join(t.TempDir(), "restored-entry")
	if _, e = backup.Restore(archive, restoredDir); e != nil {
		t.Fatal(e)
	}
	restored, restoredStore, restoredMedia := openAdminEntryFixture(t, restoredDir)
	oldOwner := owner
	oldOwner.handler = restored
	oldOwner.json("GET", jobPath, nil, 401)
	fresh := dataEntryClient{t: t, handler: restored}
	fresh = fresh.signed(fresh.json("POST", "/admin/api/login", map[string]string{"username": "entry-owner", "password": password}, 200))
	for _, id := range []string{generic.PublicID, shopify.PublicID} {
		var result importer.Preview
		fresh.decode(fresh.json("POST", "/admin/api/imports/"+id+"/execute", nil, 200), &result)
		if result.Status != "succeeded" {
			t.Fatal("restored successful import did not remain successful")
		}
	}
	fresh.decode(fresh.multipart("/admin/api/imports/csv/preview", "same.csv", dataEntryGenericCSV, map[string]string{"format": "generic"}, 200), &repeated)
	if repeated.PublicID != generic.PublicID || repeated.Status != "succeeded" {
		t.Fatal("restored same file lost original task")
	}
	dataEntryCounts(t, restoredStore, 2, 3, 3)
	dataEntryVariants(t, restoredStore)
	dataEntryCounts(t, st, 2, 3, 3)
	dataEntryVariants(t, st)
	var encrypted int
	var ciphertext string
	if e = restoredStore.DB.QueryRow(`SELECT encrypted,value FROM setting WHERE key=?`, mail.KeySMTPPassword).Scan(&encrypted, &ciphertext); e != nil || encrypted != 1 || ciphertext == secret || ciphertext == "" {
		t.Fatal("encrypted configuration lost", e)
	}
	for name, hash := range mediaFiles {
		data, e := os.ReadFile(filepath.Join(restoredMedia, strings.TrimPrefix(name, "media/")))
		if e != nil || sha256.Sum256(data) != hash {
			t.Fatal("restored media mismatch", e)
		}
	}
	original, e := os.ReadFile(archive)
	if e != nil || sha256.Sum256(original) != archiveHash {
		t.Fatal("restore modified source archive", e)
	}
	if _, e = backup.Restore(archive, restoredDir); e == nil {
		t.Fatal("restore overwrote existing destination")
	}
	if e = restoredStore.DB.PingContext(context.Background()); e != nil {
		t.Fatal(e)
	}
}
