// 游客订单访问 HTTP / Guest Order Access HTTP
// 功能：订单专用 Cookie、下单直达、邮箱验证码恢复、私密订单页与退出访问
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package storefront

import (
	"errors"
	"net/http"
	"time"

	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/order"
)

const guestAccessCookie = "kartwo_order_access"
const guestChallengeCookie = "kartwo_order_challenge"

func (h *HTTP) ConfigureGuestMail(cache *mail.Cache) {
	if cache == nil {
		h.guestMailReady = nil
		return
	}
	h.guestMailReady = func() bool { _, ready := cache.Config(); return ready }
}
func guestOrderPath(id string) string { return "/order/" + id }
func validGuestOrderID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if c != '-' && c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
func guestCookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
func (h *HTTP) setGuestCookie(w http.ResponseWriter, r *http.Request, name, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: guestOrderPath(r.PathValue("id")), HttpOnly: true, Secure: h.secureFor(r), SameSite: http.SameSiteLaxMode, MaxAge: int(ttl / time.Second), Expires: time.Now().Add(ttl).UTC()})
}
func (h *HTTP) guestAuthorized(w http.ResponseWriter, r *http.Request) (bool, error) {
	if !validGuestOrderID(r.PathValue("id")) {
		http.NotFound(w, r)
		return false, order.ErrGuestVerification
	}
	ok, err := h.order.GuestAuthorized(r.Context(), r.PathValue("id"), guestCookie(r, guestAccessCookie))
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
	}
	return ok, err
}
func (h *HTTP) renderGuestAccess(w http.ResponseWriter, r *http.Request, message string) {
	if !validGuestOrderID(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	ready := h.guestMailReady != nil && h.guestMailReady()
	h.render(w, r, h.orderAccessTmpl, map[string]any{
		"ShopName": h.shopName(r.Context()), "OrderID": r.PathValue("id"), "Message": message, "MailReady": ready,
		"SEO": seo{Title: "Verify order access — " + h.shopName(r.Context()), Description: "Verify your order email to view your order.", Canonical: h.base(r) + guestOrderPath(r.PathValue("id")), OGType: "website", NoIndex: true},
	})
}
func (h *HTTP) requestGuestAccess(w http.ResponseWriter, r *http.Request) {
	if !validGuestOrderID(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	if !parseCommerceForm(w, r) {
		return
	}
	browser := guestCookie(r, guestChallengeCookie)
	if !order.ValidGuestToken(browser) {
		var err error
		browser, err = order.NewGuestToken()
		if err != nil {
			http.Error(w, "Something went wrong", 500)
			return
		}
	}
	// Issue the same-shaped browser challenge for known, unknown and throttled
	// requests. Never reveal whether the order or submitted email exists.
	h.setGuestCookie(w, r, guestChallengeCookie, browser, order.GuestChallengeTTL)
	if h.guestMailReady != nil && h.guestMailReady() {
		if err := h.order.RequestGuestAccess(r.Context(), r.PathValue("id"), r.PostFormValue("email"), browser); err != nil {
			http.Error(w, "We could not process your request. Please try again later.", 500)
			return
		}
	}
	h.renderGuestAccess(w, r, "If the order and email match, a code will arrive shortly. Please wait at least one minute before requesting another code. Codes expire after 10 minutes.")
}
func (h *HTTP) verifyGuestAccess(w http.ResponseWriter, r *http.Request) {
	if !validGuestOrderID(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	if !parseCommerceForm(w, r) {
		return
	}
	token, err := h.order.VerifyGuestAccess(r.Context(), r.PathValue("id"), guestCookie(r, guestChallengeCookie), r.PostFormValue("code"))
	if errors.Is(err, order.ErrGuestVerification) {
		h.renderGuestAccess(w, r, "That code is invalid or expired. Please check your email or request a new code.")
		return
	}
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	h.setGuestCookie(w, r, guestAccessCookie, token, order.GuestAccessTTL)
	h.setGuestCookie(w, r, guestChallengeCookie, "", -time.Second)
	http.Redirect(w, r, guestOrderPath(r.PathValue("id")), http.StatusSeeOther)
}
func (h *HTTP) logoutGuestAccess(w http.ResponseWriter, r *http.Request) {
	if !validGuestOrderID(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	if err := h.order.RevokeGuestAccess(r.Context(), r.PathValue("id"), guestCookie(r, guestAccessCookie)); err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	h.setGuestCookie(w, r, guestAccessCookie, "", -time.Second)
	h.setGuestCookie(w, r, guestChallengeCookie, "", -time.Second)
	http.Redirect(w, r, guestOrderPath(r.PathValue("id")), http.StatusSeeOther)
}
