// 店面结算 HTTP / Storefront Checkout Handlers
// 功能：结算页（购物车摘要+收货表单）、提交下单（防超卖）、订单确认页；表单提交无需 JS
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 13:40:31
package storefront

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/order"
)

func (h *HTTP) checkoutPage(w http.ResponseWriter, r *http.Request) {
	h.renderCheckout(w, r, "")
}

// renderCheckout 渲染结算页（errMsg 非空时显示错误，保留购物车）。
func (h *HTTP) renderCheckout(w http.ResponseWriter, r *http.Request, errMsg string) {
	id, err := h.cartCtx(w, r)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	view, err := h.cart.View(r.Context(), id)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"ShopName": h.shopName,
		"Cart":     view,
		"Error":    errMsg,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title: "Checkout — " + h.shopName, Description: "Checkout", Canonical: h.base(r) + "/checkout", OGType: "website",
		},
	}
	h.render(w, h.ckoutTmpl, data)
}

func (h *HTTP) checkoutSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderCheckout(w, r, "表单解析失败")
		return
	}
	id, err := h.cartCtx(w, r)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	info := order.CheckoutInfo{
		Email:   r.PostFormValue("email"),
		Name:    r.PostFormValue("name"),
		Phone:   r.PostFormValue("phone"),
		Address: r.PostFormValue("address"),
		Country: r.PostFormValue("country"),
	}
	publicID, err := h.order.Checkout(r.Context(), id, info)
	if err != nil {
		switch {
		case errors.Is(err, order.ErrEmptyCart):
			h.renderCheckout(w, r, "Your cart is empty.")
		case errors.Is(err, order.ErrOutOfStock):
			h.renderCheckout(w, r, "Sorry, some items are no longer in stock. Please adjust your cart.")
		case errors.Is(err, order.ErrInvalidInfo):
			h.renderCheckout(w, r, "Please enter a valid email, full name and shipping address.")
		default:
			http.Error(w, "Something went wrong", http.StatusInternalServerError)
		}
		return
	}
	// 下单成功：清购物车 cookie（车已转换；下次购物自动新建）。
	http.SetCookie(w, &http.Cookie{Name: cartCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/order/"+publicID, http.StatusSeeOther)
}

func (h *HTTP) orderPage(w http.ResponseWriter, r *http.Request) {
	o, err := h.order.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"ShopName": h.shopName,
		"Order":    o,
		"Money":    h.money(r.Context()),
		"SEO": seo{
			Title: "Order — " + h.shopName, Description: "Order confirmation", Canonical: h.base(r) + "/order/" + o.PublicID, OGType: "website",
		},
	}
	h.render(w, h.orderTmpl, data)
}
