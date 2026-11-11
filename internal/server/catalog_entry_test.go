// 商品后台贯通测试 / Catalog Administration Entry Tests
// 功能：验证正式服务的分类关联、商品发布、精确价量和独立员工权限及持久事实
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 16:48:13
package server

import (
	"net/url"
	"strings"
	"testing"
)

func catalogEntryStaff(t *testing.T, owner dataEntryClient, username string, permissions []string) dataEntryClient {
	t.Helper()
	var role struct {
		ID string `json:"public_id"`
	}
	owner.decode(owner.json("POST", "/admin/api/roles", map[string]any{"name": username, "permissions": permissions}, 201), &role)
	var invite struct {
		Path string `json:"invite_path"`
	}
	owner.decode(owner.json("POST", "/admin/api/users", map[string]string{"username": username, "role_id": role.ID}, 201), &invite)
	link, err := url.Parse(invite.Path)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.Parse(link.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	anonymous := dataEntryClient{t: t, handler: owner.handler}
	password := dataEntryToken(t)
	anonymous.json("POST", "/admin/api/invites/accept", map[string]string{"token": fragment.Query().Get("token"), "password": password}, 200)
	return anonymous.signed(anonymous.json("POST", "/admin/api/login", map[string]string{"username": username, "password": password}, 200))
}

func TestCatalogAdministrationPermissionAndPersistenceFlow(t *testing.T) {
	handler, st, _ := newAdminEntryFixture(t)
	owner, _ := dataEntryOwner(t, handler)
	anonymous := dataEntryClient{t: t, handler: handler}
	var category struct {
		ID string `json:"public_id"`
	}
	owner.decode(owner.json("POST", "/admin/api/categories", map[string]any{"name": "Entry Running", "slug": "entry-running", "position": 2}, 201), &category)
	var product struct {
		ID string `json:"public_id"`
	}
	owner.decode(owner.json("POST", "/admin/api/products", map[string]any{
		"title": "Catalog Entry Tee", "slug": "catalog-entry-tee", "description": "Light running shirt", "status": "draft", "category_public_ids": []string{category.ID},
		"options": []any{map[string]any{"name": "Size", "values": []string{"S", "M"}}},
		"variants": []any{
			map[string]any{"sku": "CATALOG-S", "price_cents": 1299, "quantity": 4, "selections": []any{map[string]string{"option": "Size", "value": "S"}}},
			map[string]any{"sku": "CATALOG-M", "price_cents": 1599, "quantity": 6, "selections": []any{map[string]string{"option": "Size", "value": "M"}}},
		},
	}, 201), &product)
	productPath := "/admin/api/products/" + product.ID
	anonymous.json("GET", productPath, nil, 401)
	anonymous.send("GET", "/p/catalog-entry-tee", nil, "", 404)
	viewer := catalogEntryStaff(t, owner, "catalog-viewer", []string{"products.view"})
	priceEditor := catalogEntryStaff(t, owner, "catalog-price", []string{"products.view", "products.price"})
	stockEditor := catalogEntryStaff(t, owner, "catalog-stock", []string{"products.view", "products.inventory"})
	viewer.json("GET", productPath, nil, 200)
	viewer.json("GET", "/admin/api/categories", nil, 200)
	viewer.json("PATCH", productPath, map[string]string{"title": "forbidden"}, 403)
	viewer.json("POST", "/admin/api/categories", map[string]any{"name": "forbidden", "slug": "forbidden", "position": 0}, 403)
	viewer.json("GET", "/admin/api/settings/shop", nil, 403)
	var variantID string
	if err := st.DB.QueryRow(`SELECT public_id FROM variant WHERE sku='CATALOG-S'`).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	variantPath := "/admin/api/variants/" + variantID
	viewer.json("PATCH", variantPath+"/price", map[string]int{"price_cents": 100}, 403)
	viewer.json("PATCH", variantPath+"/inventory", map[string]int{"quantity": 100}, 403)
	priceEditor.json("PATCH", variantPath+"/inventory", map[string]int{"quantity": 100}, 403)
	stockEditor.json("PATCH", variantPath+"/price", map[string]int{"price_cents": 100}, 403)
	priceEditor.json("PATCH", variantPath+"/price", map[string]int{"price_cents": -1}, 400)
	stockEditor.json("PATCH", variantPath+"/inventory", map[string]int{"quantity": -1}, 400)
	noCSRF := priceEditor
	noCSRF.csrf = ""
	noCSRF.json("PATCH", variantPath+"/price", map[string]int{"price_cents": 100}, 403)
	priceEditor.json("PATCH", variantPath+"/price", map[string]int{"price_cents": 2550}, 200)
	stockEditor.json("PATCH", variantPath+"/inventory", map[string]int{"quantity": 9}, 200)
	priceEditor.json("PATCH", productPath, map[string]string{"title": "forbidden"}, 403)
	stockEditor.json("PATCH", productPath, map[string]string{"title": "forbidden"}, 403)
	owner.json("DELETE", "/admin/api/categories/"+category.ID, nil, 409)
	owner.json("PATCH", productPath, map[string]any{"title": "Catalog Entry Tee Updated", "description": "Saved running shirt", "status": "active", "category_public_ids": []string{category.ID}}, 200)
	var current struct {
		ID         string   `json:"public_id"`
		Slug       string   `json:"slug"`
		Title      string   `json:"title"`
		Status     string   `json:"status"`
		Categories []string `json:"category_public_ids"`
		Variants   []struct {
			ID       string `json:"public_id"`
			Price    int64  `json:"price_cents"`
			Quantity int64  `json:"quantity"`
		} `json:"variants"`
	}
	viewer.decode(viewer.json("GET", productPath, nil, 200), &current)
	if current.ID != product.ID || current.Slug != "catalog-entry-tee" || current.Title != "Catalog Entry Tee Updated" || current.Status != "active" || len(current.Categories) != 1 || current.Categories[0] != category.ID || len(current.Variants) != 2 {
		t.Fatal("product content/category contract changed")
	}
	found := false
	for _, v := range current.Variants {
		if v.ID == variantID {
			found = true
			if v.Price != 2550 || v.Quantity != 9 {
				t.Fatal("persisted cents/quantity changed")
			}
		}
	}
	if !found {
		t.Fatal("updated variant missing from readback")
	}
	page := anonymous.send("GET", "/p/catalog-entry-tee", nil, "", 200)
	if !strings.Contains(page.Body.String(), "Catalog Entry Tee Updated") {
		t.Fatal("published product unavailable in storefront")
	}
	var categories struct {
		Rows []struct {
			ID    string `json:"public_id"`
			Count int    `json:"product_count"`
		} `json:"categories"`
	}
	viewer.decode(viewer.json("GET", "/admin/api/categories", nil, 200), &categories)
	if len(categories.Rows) != 1 || categories.Rows[0].ID != category.ID || categories.Rows[0].Count != 1 {
		t.Fatal("category association/count changed")
	}
	demo := anonymous.signed(anonymous.json("POST", "/admin/api/demo-session", nil, 201))
	demo.json("GET", productPath, nil, 200)
	demo.json("PATCH", productPath, map[string]string{"title": "forbidden"}, 403)
	demo.json("PATCH", variantPath+"/price", map[string]int{"price_cents": 100}, 403)
	demo.json("PATCH", variantPath+"/inventory", map[string]int{"quantity": 100}, 403)
	var price, quantity, reserved int64
	if err := st.DB.QueryRow(`SELECT v.price_cents,i.quantity,i.reserved FROM variant v JOIN inventory i ON i.variant_id=v.id WHERE v.public_id=?`, variantID).Scan(&price, &quantity, &reserved); err != nil || price != 2550 || quantity != 9 || reserved != 0 {
		t.Fatal("denied actions changed stored price/inventory", err)
	}
	dataEntryCounts(t, st, 1, 2, 0)
}
