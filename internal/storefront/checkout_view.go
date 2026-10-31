// 多国结算视图 / International Checkout View
// 功能：保留表单、显示国家字段与权威运费快照，错误后要求重新确认且不自动下单
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 18:44:30
package storefront

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kartwo/kartwo/internal/address"
	"github.com/kartwo/kartwo/internal/order"
)

type checkoutAddressField struct {
	address.Field
	Value, Autocomplete string
	Invalid             bool
}

// Only body fields are redisplayed. Customer data is never read from a URL.
func checkoutValues(r *http.Request) map[string]string {
	values := make(map[string]string)
	for _, key := range []string{"email", "name", "phone", "country", "line1", "line2", "city", "district", "region", "postal_code", "payment_method"} {
		values[key] = r.PostForm.Get(key)
	}
	values["country"] = strings.ToUpper(strings.TrimSpace(values["country"]))
	return values
}

func checkoutError(err error) (string, string, int) {
	var field *address.FieldError
	switch {
	case errors.As(err, &field):
		return field.Message, field.Field, http.StatusUnprocessableEntity
	case errors.Is(err, order.ErrQuoteChanged):
		return "Your cart or delivery quote has changed. Review the updated total below and submit again to confirm.", "", http.StatusConflict
	case errors.Is(err, order.ErrNoShippingRule):
		return "Delivery is not available for this destination. Please select another country or contact the store.", "country", http.StatusUnprocessableEntity
	case errors.Is(err, order.ErrCartConverted):
		return "This cart has already been checked out. Please check your existing order before placing another.", "", http.StatusConflict
	case errors.Is(err, order.ErrEmptyCart):
		return "Your cart is empty.", "", http.StatusBadRequest
	case errors.Is(err, order.ErrOutOfStock):
		return "Some items are unavailable or no longer in stock. Please adjust your cart.", "", http.StatusConflict
	case errors.Is(err, order.ErrInvalidInfo):
		return "Please check your email, full name and shipping details.", "", http.StatusUnprocessableEntity
	default:
		return "We could not confirm your order or delivery quote. Please try again before proceeding to payment.", "", http.StatusInternalServerError
	}
}

func (h *HTTP) renderCheckout(w http.ResponseWriter, r *http.Request, msg string) {
	h.renderCheckoutStatus(w, r, msg, "", http.StatusOK)
}

func (h *HTTP) renderCheckoutStatus(w http.ResponseWriter, r *http.Request, msg, invalid string, status int) {
	values := checkoutValues(r)
	id, err := h.existingCartID(r)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	cart, err := h.cart.View(r.Context(), id)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	countries, err := h.order.ShippingCountries(r.Context())
	if err != nil {
		http.Error(w, "Delivery is temporarily unavailable. Please try again later.", http.StatusServiceUnavailable)
		return
	}
	// A sole existing destination can be selected up front without enabling any market.
	if r.Method == http.MethodGet && values["country"] == "" && len(countries) == 1 {
		values["country"] = countries[0].Code
	}
	selectedEnabled := false
	for _, country := range countries {
		if country.Code == values["country"] {
			selectedEnabled = true
			break
		}
	}
	var quote *order.DeliveryQuote
	if len(cart.Lines) > 0 && values["country"] != "" {
		snapshot, quoteErr := h.order.QuoteDelivery(r.Context(), id, values["country"])
		if quoteErr != nil {
			// A new failed quote must never leave an old amount or submit button active.
			quoteMsg, quoteField, quoteStatus := checkoutError(quoteErr)
			if msg == "" || quoteStatus >= 500 {
				msg, invalid, status = quoteMsg, quoteField, quoteStatus
			}
		} else if selectedEnabled {
			quote = &snapshot
		}
	}
	profile, known := address.ForCountry(values["country"])
	if !known {
		profile.Fields = []address.Field{
			{Key: "line1", Label: "Street address", Required: true, Used: true, MaxBytes: 256},
			{Key: "line2", Label: "Apartment, suite, unit (optional)", Used: true, MaxBytes: 256},
			{Key: "city", Label: "City / locality", MaxBytes: 128},
			{Key: "district", Label: "District / neighborhood", MaxBytes: 128},
			{Key: "region", Label: "State / province / region", MaxBytes: 128},
			{Key: "postal_code", Label: "Postal code", MaxBytes: 32},
		}
	}
	autocomplete := map[string]string{"line1": "address-line1", "line2": "address-line2", "city": "address-level2", "district": "address-level3", "region": "address-level1", "postal_code": "postal-code"}
	fields := []checkoutAddressField{}
	for _, field := range profile.Fields {
		// Keep entered fields visible after switching country, including ones it normally omits.
		if field.Used || field.Required || values[field.Key] != "" {
			fields = append(fields, checkoutAddressField{Field: field, Value: values[field.Key], Autocomplete: autocomplete[field.Key], Invalid: invalid == field.Key})
		}
	}
	var methods []string
	if h.pay != nil {
		methods = h.pay.AvailableMethods(r.Context())
	}
	chosen := ""
	for _, method := range methods {
		if method == values["payment_method"] {
			chosen = method
			break
		}
	}
	if chosen == "" && len(methods) > 0 {
		chosen = methods[0]
	}
	data := map[string]any{
		"ShopName": h.shopName(r.Context()), "Cart": cart, "Error": msg, "InvalidField": invalid,
		"Values": values, "Fields": fields, "PostalExample": profile.PostalExample,
		"Countries": countries, "SelectedEnabled": selectedEnabled, "Methods": methods, "ChosenMethod": chosen,
		"Quote": quote, "Money": h.money(r.Context()), "HTTPStatus": status,
		"CanPlace": quote != nil && status < 500,
		"SEO":      seo{Title: "Checkout — " + h.shopName(r.Context()), Description: "Checkout", NoIndex: true, Canonical: h.base(r) + "/checkout", OGType: "website"},
	}
	if quote != nil {
		data["Money"] = moneyFunc(quote.Currency)
	}
	h.render(w, r, h.ckoutTmpl, data)
}

func (h *HTTP) checkoutJS(w http.ResponseWriter, _ *http.Request) {
	body, err := tmplFS.ReadFile("static/checkout.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(body)
}
