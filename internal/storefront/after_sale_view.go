// 售后私密页面 / Private After Sale View
// 功能：冻结订单币种、展示政策和事件、失败后保留正文草稿与原键；数据库不可读时提供独立恢复页
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 21:26:50
package storefront

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/policy"
)

var afterSaleStates = map[string]string{
	"requested": "Awaiting review", "awaiting_return": "Return approved — awaiting parcel", "returned": "Return received — inspection pending",
	"approved": "Refund approved — awaiting processing", "completed": "Refund processed", "rejected": "Request declined", "withdrawn": "Request withdrawn",
}
var afterSaleActions = map[string]string{
	"request": "Request submitted", "approve": "Request approved", "reject": "Request declined", "receive_return": "Return parcel received", "complete": "Verified refund recorded", "withdraw": "Request withdrawn", "return_tracking": "Return tracking provided",
}

type afterSaleEventCard struct {
	order.AfterSaleEvent
	Label string
}
type afterSaleCard struct {
	Case                  order.AfterSaleCase
	Form                  afterSaleForm
	StateLabel, KindLabel string
	CanWithdraw, CanTrack bool
	Events                []afterSaleEventCard
}
type afterSaleHidden struct{ Name, Value string }

func (h *HTTP) renderAfterSales(w http.ResponseWriter, r *http.Request, status int, message string, draft *afterSaleForm, uncertain, review bool) {
	id := r.PathValue("id")
	snapshot, err := h.order.GuestAfterSales(r.Context(), id, guestCookie(r, guestAccessCookie))
	if errors.Is(err, order.ErrGuestVerification) || errors.Is(err, sql.ErrNoRows) {
		h.renderGuestAccess(w, r, "Please verify your order email again to continue.")
		return
	}
	readError := err != nil
	policyError := false
	var current *order.Order
	var terms order.AfterSaleTerms
	if !readError {
		current, err = h.order.Get(r.Context(), id)
		readError = err != nil
	}
	if !readError {
		profile, configured, err := policy.LoadProfile(r.Context(), h.settings)
		policyError = err != nil
		if configured {
			terms = order.AfterSaleTerms{Configured: true, WindowDays: profile.ReturnWindowDays, ShippingPayer: profile.ReturnShippingPayer, SupportEmail: profile.SupportEmail}
		}
	}
	if readError && message == "" {
		message = "The latest records could not be loaded. Please try again shortly."
	}
	if readError {
		status = http.StatusInternalServerError
		snapshot = order.AfterSaleSnapshot{}
		current = nil
	}
	form := afterSaleForm{Operation: "request", Kind: "refund", RequestKey: uuid.Must(uuid.NewV7()).String()}
	if draft != nil && draft.Operation == "request" {
		form = *draft
	}
	cards := make([]afterSaleCard, 0, len(snapshot.Cases))
	for _, c := range snapshot.Cases {
		card := afterSaleCard{Case: c, StateLabel: afterSaleStates[c.State], KindLabel: "Refund only", Form: afterSaleForm{CaseID: c.PublicID, RequestKey: uuid.Must(uuid.NewV7()).String(), ExpectedVersion: strconv.FormatInt(c.Version, 10), Carrier: c.ReturnCarrier, Number: c.ReturnTrackingNumber}}
		if c.Kind == "return_refund" {
			card.KindLabel = "Return and refund"
		}
		if draft != nil && draft.CaseID == c.PublicID {
			card.Form = *draft
		}
		card.CanWithdraw = c.State == "requested" || c.State == "awaiting_return" && c.ReturnTrackingNumber == ""
		card.CanTrack = c.Kind == "return_refund" && c.State == "awaiting_return" && c.Version < order.AfterSaleEventLimit-3
		for _, e := range c.Events {
			card.Events = append(card.Events, afterSaleEventCard{AfterSaleEvent: e, Label: afterSaleActions[e.Action]})
		}
		cards = append(cards, card)
	}
	var recovery []afterSaleHidden
	recoveryPath := afterSalePath(id)
	if uncertain && draft != nil {
		recovery = []afterSaleHidden{{"request_key", draft.RequestKey}}
		if draft.Operation == "request" {
			recovery = append(recovery, afterSaleHidden{"kind", draft.Kind}, afterSaleHidden{"amount", draft.Amount}, afterSaleHidden{"reason", draft.Reason})
		} else {
			recoveryPath += "/" + draft.CaseID + "/actions"
			recovery = append(recovery, afterSaleHidden{"action", draft.Operation}, afterSaleHidden{"expected_version", draft.ExpectedVersion}, afterSaleHidden{"message", draft.Message}, afterSaleHidden{"carrier", draft.Carrier}, afterSaleHidden{"tracking_number", draft.Number})
		}
	}
	data := map[string]any{
		"OrderID": id, "Order": current, "Snapshot": snapshot, "Terms": terms, "Cards": cards, "Form": form,
		"RequestPath": afterSalePath(id), "Message": message, "ReadError": readError, "Uncertain": uncertain, "NeedsReview": review,
		"Draft": draft, "Recovery": recovery, "RecoveryPath": recoveryPath, "HTTPStatus": status,
		"PolicyUnavailable": policyError,
		"ShowRequestForm":   snapshot.CanRequestRefund || draft != nil && draft.Operation == "request",
		"Disabled":          uncertain || review || readError, "RequestDisabled": uncertain || review || readError || policyError || !snapshot.CanRequestRefund,
		"SEO": seo{Title: "Returns and refunds — " + h.shopNameFallback, Description: "Private order requests", NoIndex: true, Canonical: h.base(r) + afterSalePath(id), OGType: "website"},
	}
	if current != nil {
		data["Money"] = moneyFunc(current.Currency)
	}
	if !readError {
		data["ShopName"] = h.shopName(r.Context())
		if err := h.preparePage(r.Context(), data); err == nil {
			writePage(w, h.afterSaleTmpl, data)
			return
		}
		data["HTTPStatus"], data["ReadError"], data["Disabled"], data["RequestDisabled"] = http.StatusInternalServerError, true, true, true
		data["Cards"], data["Order"], data["Snapshot"], data["Terms"] = nil, nil, order.AfterSaleSnapshot{}, order.AfterSaleTerms{}
		data["NavigationCategories"], data["FooterPages"], data["ShopLogoURL"], data["BlogVisible"] = nil, nil, "", false
		data["Message"] = "The latest records could not be fully loaded. Keep the original details and try again shortly."
		data["ShowRequestForm"] = draft != nil && draft.Operation == "request"
	}
	// Do not lose the original POST when the shared renderer cannot read its
	// navigation/settings either. This fallback renders no unverified records.
	data["ShopName"] = h.shopNameFallback
	writePage(w, h.afterSaleTmpl, data)
}
