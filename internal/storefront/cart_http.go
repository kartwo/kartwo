// 店面购物车 HTTP / Storefront Cart Handlers
// 功能：匿名购物车 cookie、加/改/删行、购物车页与 JSON、cart.js（渐进增强）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 12:40:00
package storefront

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/cart"
)

const cartCookie = "kartwo_cart"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体非法")
		return false
	}
	return true
}

// cartCtx 取（或新建）当前请求的购物车，必要时下发 cookie，返回 cartID。
func (h *HTTP) cartCtx(w http.ResponseWriter, r *http.Request) (int64, error) {
	var token string
	if c, err := r.Cookie(cartCookie); err == nil {
		token = c.Value
	}
	id, tok, err := h.cart.GetOrCreate(r.Context(), token)
	if err != nil {
		return 0, err
	}
	if tok != token {
		http.SetCookie(w, &http.Cookie{
			Name: cartCookie, Value: tok, Path: "/", HttpOnly: true,
			Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: 60 * 60 * 24 * 30,
		})
	}
	return id, nil
}

func (h *HTTP) cartAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Variant  string `json:"variant"`
		Quantity int64  `json:"quantity"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	id, err := h.cartCtx(w, r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	if err := h.cart.AddItem(r.Context(), id, req.Variant, req.Quantity); err != nil {
		h.writeCartErr(w, err)
		return
	}
	h.respondCount(w, r, id)
}

func (h *HTTP) cartSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Quantity int64 `json:"quantity"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	id, err := h.cartCtx(w, r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	if err := h.cart.SetQty(r.Context(), id, r.PathValue("vid"), req.Quantity); err != nil {
		h.writeCartErr(w, err)
		return
	}
	h.respondCount(w, r, id)
}

func (h *HTTP) cartRemove(w http.ResponseWriter, r *http.Request) {
	id, err := h.cartCtx(w, r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	if err := h.cart.RemoveItem(r.Context(), id, r.PathValue("vid")); err != nil {
		h.writeCartErr(w, err)
		return
	}
	h.respondCount(w, r, id)
}

// cartData 返回购物车 JSON（供 JS 刷新抽屉/角标）。
func (h *HTTP) cartData(w http.ResponseWriter, r *http.Request) {
	id, err := h.cartCtx(w, r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	view, err := h.cart.View(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	writeJSON(w, http.StatusOK, cartJSON(view, h.cur(r.Context())))
}

// cartPage 服务端渲染购物车页。
func (h *HTTP) cartPage(w http.ResponseWriter, r *http.Request) {
	id, err := h.cartCtx(w, r)
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	view, err := h.cart.View(r.Context(), id)
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	canonical := h.base(r) + "/cart"
	data := map[string]any{
		"ShopName": h.shopName,
		"Cart":     view,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title: "购物车 — " + h.shopName, Description: "购物车", Canonical: canonical, OGType: "website",
		},
	}
	h.render(w, h.cartTmpl, data)
}

func (h *HTTP) respondCount(w http.ResponseWriter, r *http.Request, cartID int64) {
	n, err := h.cart.Count(r.Context(), cartID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n})
}

func (h *HTTP) cartJS(w http.ResponseWriter, _ *http.Request) {
	b, err := tmplFS.ReadFile("static/cart.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(b)
}

func (h *HTTP) writeCartErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cart.ErrVariantNotFound):
		writeErr(w, http.StatusNotFound, "商品规格不存在")
	case errors.Is(err, cart.ErrInvalidQty):
		writeErr(w, http.StatusBadRequest, "数量非法")
	default:
		writeErr(w, http.StatusInternalServerError, "内部错误")
	}
}

func cartJSON(v *cart.CartView, currency string) map[string]any {
	money := moneyFunc(currency)
	lines := make([]map[string]any, 0, len(v.Lines))
	for _, l := range v.Lines {
		opts := make([]map[string]string, 0, len(l.Options))
		for _, o := range l.Options {
			opts = append(opts, map[string]string{"name": o.Name, "value": o.Value})
		}
		lines = append(lines, map[string]any{
			"variant": l.VariantPublicID, "title": l.ProductTitle, "slug": l.ProductSlug,
			"sku": l.SKU, "options": opts, "unit": money(l.UnitCents),
			"quantity": l.Quantity, "line": money(l.LineCents), "thumb": l.ThumbURL,
		})
	}
	return map[string]any{"count": v.Count, "total": money(v.TotalCents), "lines": lines}
}
