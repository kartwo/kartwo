// 后台权限注册表 / Admin Permission Registry
// 功能：固定权限词汇、员工角色授权和逐路由服务端校验
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-24 10:10:00
package admin

import (
	"encoding/json"
	"net/http"
	"sort"
)

// staffRoutes 是唯一的员工接口授权表。未列出的后台接口默认拒绝员工访问。
var staffRoutes = map[string]string{
	"GET /admin/api/dashboard":                      "dashboard.view",
	"GET /admin/api/diagnostics":                    "diagnostics.view",
	"GET /admin/api/audit-events":                   "audit.view",
	"GET /admin/api/contact/messages":               "contact.view",
	"GET /admin/api/contact/messages/{id}":          "contact.view",
	"PATCH /admin/api/contact/messages/{id}":        "contact.manage",
	"GET /admin/api/settings/contact":               "contact.view",
	"GET /admin/api/products":                       "products.view",
	"GET /admin/api/products/{id}":                  "products.view",
	"POST /admin/api/products":                      "products.create",
	"PATCH /admin/api/products/{id}":                "products.edit",
	"PATCH /admin/api/products/{id}/featured":       "products.publish",
	"DELETE /admin/api/products/{id}":               "products.delete",
	"PATCH /admin/api/variants/{id}/inventory":      "products.inventory",
	"PATCH /admin/api/variants/{id}/price":          "products.price",
	"GET /admin/api/products/{id}/media":            "products.view",
	"POST /admin/api/products/{id}/media":           "products.media",
	"DELETE /admin/api/media/{id}":                  "products.media",
	"PATCH /admin/api/media/{id}":                   "products.media",
	"GET /admin/api/categories":                     "products.view",
	"POST /admin/api/categories":                    "categories.manage",
	"PATCH /admin/api/categories/{id}":              "categories.manage",
	"DELETE /admin/api/categories/{id}":             "categories.manage",
	"GET /admin/api/content-pages":                  "content.manage",
	"GET /admin/api/content-pages/{id}":             "content.manage",
	"POST /admin/api/content-pages":                 "content.manage",
	"PATCH /admin/api/content-pages/{id}":           "content.manage",
	"DELETE /admin/api/content-pages/{id}":          "content.manage",
	"POST /admin/api/content-pages/generate-footer": "content.manage",
	"GET /admin/api/blog/posts":                     "blog.view",
	"GET /admin/api/blog/posts/{id}":                "blog.view",
	"POST /admin/api/blog/posts":                    "blog.manage",
	"PUT /admin/api/blog/posts/{id}":                "blog.manage",
	"DELETE /admin/api/blog/posts/{id}":             "blog.manage",
	"POST /admin/api/blog/images":                   "blog.manage",
	"POST /admin/api/blog/preview":                  "blog.view",
	"GET /admin/api/settings/blog":                  "blog.view",
	"POST /admin/api/imports/csv/preview":           "imports.manage",
	"POST /admin/api/imports/{id}/execute":          "imports.manage",
	"GET /admin/api/imports/{id}":                   "imports.manage",
	"GET /admin/api/orders":                         "orders.view",
	"GET /admin/api/orders/{id}":                    "orders.view",
	"GET /admin/api/orders/export":                  "orders.export",
	"POST /admin/api/orders/{id}/fulfill":           "orders.fulfill",
	"POST /admin/api/orders/{id}/refund":            "orders.refund",
	"GET /admin/api/settings/shop":                  "settings.shop",
	"PUT /admin/api/settings/shop":                  "settings.shop",
	"POST /admin/api/settings/shop/logo":            "settings.shop",
	"DELETE /admin/api/settings/shop/logo":          "settings.shop",
	"GET /admin/api/settings/policy-profile":        "settings.policy",
	"PUT /admin/api/settings/policy-profile":        "settings.policy",
	"GET /admin/api/settings/shipping/countries":    "settings.shipping",
	"PUT /admin/api/settings/shipping/countries":    "settings.shipping",
	"PUT /admin/api/settings/shipping/default":      "settings.shipping",
	"GET /admin/api/settings/shipping":              "settings.shipping",
	"POST /admin/api/settings/shipping":             "settings.shipping",
	"PATCH /admin/api/settings/shipping/{id}":       "settings.shipping",
	"DELETE /admin/api/settings/shipping/{id}":      "settings.shipping",
	"GET /admin/api/settings/domain":                "settings.domain",
	"PUT /admin/api/settings/domain":                "settings.domain",
	"GET /admin/api/settings/translation":           "translation.manage",
	"PUT /admin/api/settings/translation":           "translation.manage",
	"POST /admin/api/translation/text":              "translation.manage",
	"GET /admin/api/settings/market":                "settings.shop",
	"GET /admin/api/markets":                        "settings.shop",
	"PUT /admin/api/settings/market":                "settings.shop",
	"GET /admin/api/users":                          "users.manage",
	"POST /admin/api/users":                         "users.manage",
	"PATCH /admin/api/users/{id}":                   "users.manage",
	"POST /admin/api/users/{id}/revoke-sessions":    "sessions.manage",
	"GET /admin/api/roles":                          "roles.manage",
	"POST /admin/api/roles":                         "roles.manage",
	"PATCH /admin/api/roles/{id}":                   "roles.manage",
	"DELETE /admin/api/roles/{id}":                  "roles.manage",

	"POST /admin/api/orders/{id}/refunds":                  "orders.refund",
	"POST /admin/api/orders/{id}/refunds/{refund}/refresh": "orders.refund",
}

// ownerOnlyPermissions 不可授予员工，即使数据库被错误写入也不产生权限。
var ownerOnlyPermissions = map[string]bool{
	"users.manage": true, "roles.manage": true, "sessions.manage": true,
	"orders.refund": true, "orders.export": true,
	"translation.manage": true,
}

var extraPermissions = map[string]bool{
	"orders.customer_data": true,
}

func parsePermissions(raw string) (map[string]bool, error) {
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, name := range list {
		if validPermission(name) && !ownerOnlyPermissions[name] {
			out[name] = true
		}
	}
	return out, nil
}

func validPermission(name string) bool {
	if extraPermissions[name] {
		return true
	}
	for _, allowed := range staffRoutes {
		if allowed == name {
			return true
		}
	}
	return false
}

func permissionsList(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (h *HTTP) allowStaff(w http.ResponseWriter, r *http.Request, ac *AuthContext) bool {
	if ac.Role != "staff" {
		return true
	}
	if r.Pattern == "GET /admin/api/me" || r.Pattern == "POST /admin/api/logout" || r.Pattern == "GET /admin/api/account" || r.Pattern == "PUT /admin/api/account/password" {
		return true
	}
	permission := staffRoutes[r.Method+" "+r.URL.Path]
	if permission == "" {
		permission = staffRoutes[r.Pattern]
	}
	if permission == "" || !ac.Permissions[permission] || ownerOnlyPermissions[permission] {
		writeErr(w, http.StatusForbidden, "当前账号没有此操作权限")
		return false
	}
	if permission == "orders.fulfill" && !ac.Permissions["orders.customer_data"] {
		writeErr(w, http.StatusForbidden, "发货需要客户信息权限")
		return false
	}
	return true
}
