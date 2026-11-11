// 顾客售后表单 / Guest After Sale Forms
// 功能：私密订单内申请退款/退货、退回运单和撤回；错误保留原请求且不以URL传递资料
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 21:26:50
package storefront

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/order"
)

type afterSaleForm struct {
	Operation, CaseID, RequestKey, ExpectedVersion string
	Kind, Amount, Reason, Message, Carrier, Number string
}

var afterSaleAmount = regexp.MustCompile(`^[0-9]{1,17}(\.[0-9]{1,2})?$`)

// Exact decimal-to-integer conversion; no floats, rounding or exponent syntax.
func afterSaleCents(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if !afterSaleAmount.MatchString(value) {
		return 0, false
	}
	whole, fraction, _ := strings.Cut(value, ".")
	dollars, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	var cents int64
	if fraction != "" {
		if len(fraction) == 1 {
			fraction += "0"
		}
		cents, _ = strconv.ParseInt(fraction, 10, 64)
	}
	if dollars > math.MaxInt64/100 || dollars == math.MaxInt64/100 && cents > math.MaxInt64%100 {
		return 0, false
	}
	total := dollars*100 + cents
	return total, total > 0
}

func afterSaleNonce(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.String() == value
}

func afterSalePath(id string) string { return guestOrderPath(id) + "/after-sales" }

func (h *HTTP) afterSalePage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	allowed, err := h.guestAuthorized(w, r)
	if err != nil {
		return
	}
	if !allowed {
		h.renderGuestAccess(w, r, "")
		return
	}
	h.renderAfterSales(w, r, http.StatusOK, "", nil, false, false)
}

func (h *HTTP) createAfterSale(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	allowed, err := h.guestAuthorized(w, r)
	if err != nil {
		return
	}
	if !allowed {
		h.renderGuestAccess(w, r, "")
		return
	}
	if !parseCommerceForm(w, r) {
		return
	}
	form := &afterSaleForm{Operation: "request", RequestKey: r.PostForm.Get("request_key"), Kind: r.PostForm.Get("kind"), Amount: r.PostForm.Get("amount"), Reason: r.PostForm.Get("reason")}
	amount, valid := afterSaleCents(form.Amount)
	if !valid {
		h.renderAfterSales(w, r, http.StatusUnprocessableEntity, "Enter a positive amount with no more than two decimal places.", form, false, !afterSaleNonce(form.RequestKey))
		return
	}
	_, err = h.order.CreateAfterSale(ctx, r.PathValue("id"), guestCookie(r, guestAccessCookie), order.AfterSaleInput{Key: form.RequestKey, Kind: form.Kind, Reason: form.Reason, AmountCents: amount})
	if err != nil {
		h.afterSaleFailure(w, r, form, err)
		return
	}
	http.Redirect(w, r, afterSalePath(r.PathValue("id")), http.StatusSeeOther)
}

func (h *HTTP) updateAfterSale(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	allowed, err := h.guestAuthorized(w, r)
	if err != nil {
		return
	}
	if !allowed {
		h.renderGuestAccess(w, r, "")
		return
	}
	if !parseCommerceForm(w, r) {
		return
	}
	form := &afterSaleForm{Operation: r.PostForm.Get("action"), CaseID: r.PathValue("case"), RequestKey: r.PostForm.Get("request_key"), ExpectedVersion: r.PostForm.Get("expected_version"), Message: r.PostForm.Get("message"), Carrier: r.PostForm.Get("carrier"), Number: r.PostForm.Get("tracking_number")}
	version, err := strconv.ParseInt(form.ExpectedVersion, 10, 64)
	if err != nil || version < 1 || !afterSaleNonce(form.CaseID) || (form.Operation != "withdraw" && form.Operation != "return_tracking") {
		h.renderAfterSales(w, r, http.StatusUnprocessableEntity, "This action is invalid. Review the latest request before trying again.", form, false, true)
		return
	}
	_, err = h.order.UpdateGuestAfterSale(ctx, r.PathValue("id"), form.CaseID, guestCookie(r, guestAccessCookie), order.AfterSaleAction{Key: form.RequestKey, ExpectedVersion: version, Action: form.Operation, Message: form.Message, Carrier: form.Carrier, TrackingNumber: form.Number})
	if err != nil {
		h.afterSaleFailure(w, r, form, err)
		return
	}
	http.Redirect(w, r, afterSalePath(r.PathValue("id")), http.StatusSeeOther)
}

func (h *HTTP) afterSaleFailure(w http.ResponseWriter, r *http.Request, form *afterSaleForm, err error) {
	if errors.Is(err, order.ErrGuestVerification) {
		h.renderGuestAccess(w, r, "Please verify your order email again to continue.")
		return
	}
	message, status, uncertain, review := "We could not confirm the result. Continue the original request below; do not create another request.", http.StatusInternalServerError, true, false
	switch {
	case errors.Is(err, sql.ErrNoRows):
		message, status, uncertain, review = "That request is not available. Review your latest requests.", http.StatusNotFound, false, true
	case errors.Is(err, order.ErrAfterSaleInvalid):
		message, status, uncertain = "Check the request type, reason, amount or return tracking details.", http.StatusUnprocessableEntity, false
		review = !afterSaleNonce(form.RequestKey)
	case errors.Is(err, order.ErrAfterSaleChanged):
		message, status, uncertain, review = "This request has changed or an active request already exists. Review the latest records before starting another action.", http.StatusConflict, false, true
	case errors.Is(err, order.ErrAfterSaleIneligible):
		message, status, uncertain = "This request is not available for the current order or remaining amount. Check the latest records or contact the store.", http.StatusUnprocessableEntity, false
	case errors.Is(err, order.ErrAfterSaleLimit):
		message, status, uncertain, review = "The request history limit has been reached. Please contact the store for help.", http.StatusConflict, false, true
	}
	h.renderAfterSales(w, r, status, message, form, uncertain, review)
}
