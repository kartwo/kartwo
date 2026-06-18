// 商品 CRUD HTTP 接口 / Product CRUD Handlers
// 功能：商品/分类/变体库存的 Admin JSON API（均需鉴权，写操作经 CSRF 中间件）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 00:00:00
package admin

import (
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/catalog"
)

// ---- 请求/响应 DTO（snake_case JSON）----

type optionDTO struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type selectionDTO struct {
	Option string `json:"option"`
	Value  string `json:"value"`
}

type variantDTO struct {
	SKU        string         `json:"sku"`
	PriceCents int64          `json:"price_cents"`
	Quantity   int64          `json:"quantity"`
	Selections []selectionDTO `json:"selections"`
}

type createProductReq struct {
	Title             string       `json:"title"`
	Slug              string       `json:"slug"`
	Description       string       `json:"description"`
	Status            string       `json:"status"`
	Options           []optionDTO  `json:"options"`
	Variants          []variantDTO `json:"variants"`
	CategoryPublicIDs []string     `json:"category_public_ids"`
}

func (h *HTTP) createProduct(w http.ResponseWriter, r *http.Request) {
	var req createProductReq
	if !readJSON(w, r, &req) {
		return
	}
	in := catalog.ProductInput{
		Title: req.Title, Slug: req.Slug, Description: req.Description, Status: req.Status,
		CategoryPublicIDs: req.CategoryPublicIDs,
	}
	for _, o := range req.Options {
		in.Options = append(in.Options, catalog.OptionInput{Name: o.Name, Values: o.Values})
	}
	for _, v := range req.Variants {
		vi := catalog.VariantInput{SKU: v.SKU, PriceCents: v.PriceCents, Quantity: v.Quantity}
		for _, s := range v.Selections {
			vi.Selections = append(vi.Selections, catalog.Selection{Option: s.Option, Value: s.Value})
		}
		in.Variants = append(in.Variants, vi)
	}

	publicID, err := h.cat.CreateProduct(r.Context(), in)
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"public_id": publicID})
}

func (h *HTTP) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.cat.ListProducts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	out := make([]map[string]any, 0, len(products))
	for _, p := range products {
		out = append(out, map[string]any{
			"public_id": p.PublicID, "title": p.Title, "slug": p.Slug, "status": p.Status, "updated_at": p.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": out})
}

func (h *HTTP) getProduct(w http.ResponseWriter, r *http.Request) {
	d, err := h.cat.GetProduct(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	variants := make([]map[string]any, 0, len(d.Variants))
	for _, v := range d.Variants {
		opts := make([]map[string]string, 0, len(v.Options))
		for _, o := range v.Options {
			opts = append(opts, map[string]string{"name": o.Name, "value": o.Value})
		}
		variants = append(variants, map[string]any{
			"public_id": v.PublicID, "sku": v.SKU, "price_cents": v.PriceCents, "quantity": v.Quantity, "options": opts,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"public_id": d.PublicID, "title": d.Title, "slug": d.Slug,
		"description": d.Description, "status": d.Status, "variants": variants,
	})
}

func (h *HTTP) updateProduct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.cat.UpdateProduct(r.Context(), r.PathValue("id"), req.Title, req.Description, req.Status); err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) deleteProduct(w http.ResponseWriter, r *http.Request) {
	if err := h.cat.DeleteProduct(r.Context(), r.PathValue("id")); err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) setVariantInventory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Quantity int64 `json:"quantity"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.cat.SetVariantInventory(r.Context(), r.PathValue("id"), req.Quantity); err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cat.ListCategories(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	out := make([]map[string]any, 0, len(cats))
	for _, c := range cats {
		out = append(out, map[string]any{"public_id": c.PublicID, "name": c.Name, "slug": c.Slug})
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

func (h *HTTP) createCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	publicID, err := h.cat.CreateCategory(r.Context(), req.Name, req.Slug)
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"public_id": publicID})
}

// writeCatalogErr 将领域错误映射为合适的 HTTP 状态。
func (h *HTTP) writeCatalogErr(w http.ResponseWriter, err error) {
	var ve *catalog.ValidationError
	switch {
	case errors.As(err, &ve):
		writeErr(w, http.StatusBadRequest, ve.Msg)
	case errors.Is(err, catalog.ErrNotFound):
		writeErr(w, http.StatusNotFound, "资源不存在")
	default:
		writeErr(w, http.StatusInternalServerError, "内部错误")
	}
}
