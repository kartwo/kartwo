// 备份设置原子保存测试 / Atomic Backup Settings Tests
// 功能：无效字段或数据库失败时完整保留原配置和加密密码，不误记成功
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 17:18:58
package admin

import (
	"crypto/rand"
	"net/http"
	"reflect"
	"testing"
)

func TestBackupSettingsAtomicSave(t *testing.T) {
	h, mux := newHTTP(t)
	sess, csrf := loginAndCookies(t, mux)
	cookies := []*http.Cookie{sess}
	password := rand.Text()
	initial := `{"interval":"24h","retention":"7","webdav_enabled":false,"webdav_url":"https://backup.example.invalid","webdav_path":"/original","webdav_username":"original","webdav_password":"` + password + `"}`
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/backup", initial, cookies, csrf); got.StatusCode != 200 {
		t.Fatalf("initial save status %d", got.StatusCode)
	}
	snapshot := func() map[string]string {
		t.Helper()
		rows, err := h.svc.db.Query(`SELECT key,value FROM setting WHERE key LIKE 'backup.%' ORDER BY key`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		result := map[string]string{}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				t.Fatal(err)
			}
			result[key] = value
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	if len(before) != 7 {
		t.Fatalf("expected seven stored fields, got %d", len(before))
	}
	var auditBefore int
	if err := h.svc.db.QueryRow(`SELECT count(*) FROM audit_event WHERE action='backup.settings_update'`).Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func() {
		t.Helper()
		if !reflect.DeepEqual(snapshot(), before) {
			t.Fatal("failed save partially changed backup configuration")
		}
		var count int
		if err := h.svc.db.QueryRow(`SELECT count(*) FROM audit_event WHERE action='backup.settings_update'`).Scan(&count); err != nil || count != auditBefore {
			t.Fatalf("failed save changed success audit count: %d, err %v", count, err)
		}
	}
	for _, body := range []string{
		`{"interval":"48h","retention":"0"}`,
		`{"interval":"48h","retention":"5","webdav_enabled":true,"webdav_url":"http://backup.example.invalid"}`,
		`{"interval":"48h","retention":"5","webdav_enabled":true,"webdav_path":"relative"}`,
	} {
		if got := doJSON(t, mux, "PUT", "/admin/api/settings/backup", body, cookies, csrf); got.StatusCode != 400 {
			t.Fatalf("invalid save status %d", got.StatusCode)
		}
		assertUnchanged()
	}
	// A late database failure must roll back both earlier fields and a new encrypted password.
	if _, err := h.svc.db.Exec(`CREATE TRIGGER reject_backup_password BEFORE UPDATE ON setting WHEN NEW.key='backup.webdav.password' BEGIN SELECT RAISE(ABORT,'injected backup write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/backup", `{"interval":"48h","retention":"5","webdav_username":"changed","webdav_password":"`+rand.Text()+`"}`, cookies, csrf); got.StatusCode != 500 {
		t.Fatalf("database failure status %d", got.StatusCode)
	}
	assertUnchanged()
	if _, err := h.svc.db.Exec(`DROP TRIGGER reject_backup_password`); err != nil {
		t.Fatal(err)
	}
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/backup", `{"interval":"48h","retention":"5","webdav_password":""}`, cookies, csrf); got.StatusCode != 200 {
		t.Fatalf("valid save status %d", got.StatusCode)
	}
	after := snapshot()
	if after["backup.interval"] != "48h" || after["backup.retention"] != "5" || after["backup.webdav.password"] != before["backup.webdav.password"] {
		t.Fatal("valid save did not preserve the blank-password contract")
	}
}
