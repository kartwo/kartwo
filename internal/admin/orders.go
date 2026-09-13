// 后台订单 HTTP / Admin Orders Handlers
// 功能：订单列表/详情 + 整单全额退款（鉴权+CSRF+对象级）；退款经支付编排，订单转 refunded
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-22 11:20:11
package admin

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/payment"
)

func (h *HTTP) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.orders.AdminList(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, o := range rows {
		email := o.Email
		if isDemoRequest(r) {
			email = "已隐藏（公开演示）"
		}
		out = append(out, map[string]any{
			"public_id": o.PublicID, "status": o.Status, "email": email,
			"currency": o.Currency, "total_cents": o.TotalCents,
			"payment_provider": o.PaymentProvider, "created_at": o.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": out})
}

// exportOrdersCSV 下载订单履约 CSV；仅管理员可访问，且不含支付平台交易参考号。
func (h *HTTP) exportOrdersCSV(w http.ResponseWriter, r *http.Request) {
	filter, err := parseOrderExportFilter(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := h.orders.AdminExport(r.Context(), filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "导出订单失败")
		return
	}

	var body bytes.Buffer
	body.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM，确保常见表格软件正确识别中文表头。
	writer := csv.NewWriter(&body)
	if err := writer.Write([]string{"订单号", "订单状态", "客户邮箱", "收件人", "联系电话", "收货地址", "收货国家", "币种", "商品小计(分)", "订单总额(分)", "支付方式", "下单时间"}); err != nil {
		writeErr(w, http.StatusInternalServerError, "导出订单失败")
		return
	}
	for _, row := range rows {
		if err := writer.Write([]string{
			csvCell(row.PublicID), csvCell(row.Status), csvCell(row.Email), csvCell(row.ShipName),
			csvCell(row.ShipPhone), csvCell(row.ShipAddress), csvCell(row.ShipCountry), csvCell(row.Currency),
			strconv.FormatInt(row.SubtotalCents, 10), strconv.FormatInt(row.TotalCents, 10),
			csvCell(row.PaymentProvider), csvCell(row.CreatedAt),
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, "导出订单失败")
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeErr(w, http.StatusInternalServerError, "导出订单失败")
		return
	}

	h.recordAudit(r, authFrom(r.Context()).AdminID, "order.export", "orders", "orders")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=kartwo-orders.csv")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body.Bytes())
}

// parseOrderExportFilter 解析可选状态与 UTC 日期范围；结束日期为整日包含的次日零点。
func parseOrderExportFilter(r *http.Request) (order.OrderExportFilter, error) {
	filter := order.OrderExportFilter{Status: strings.TrimSpace(r.URL.Query().Get("status"))}
	if filter.Status != "" {
		switch filter.Status {
		case "pending", "paid", "refunded", "cancelled", "fulfilled":
		default:
			return order.OrderExportFilter{}, errors.New("订单状态筛选值无效")
		}
	}
	parseDate := func(key string, end bool) (string, error) {
		raw := strings.TrimSpace(r.URL.Query().Get(key))
		if raw == "" {
			return "", nil
		}
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return "", errors.New("日期格式应为 YYYY-MM-DD")
		}
		if end {
			value = value.AddDate(0, 0, 1)
		}
		return value.UTC().Format("2006-01-02T15:04:05.000Z"), nil
	}
	var err error
	if filter.From, err = parseDate("from", false); err != nil {
		return order.OrderExportFilter{}, err
	}
	if filter.To, err = parseDate("to", true); err != nil {
		return order.OrderExportFilter{}, err
	}
	if filter.From != "" && filter.To != "" && filter.From >= filter.To {
		return order.OrderExportFilter{}, errors.New("结束日期不能早于开始日期")
	}
	return filter, nil
}

// csvCell 防止用户可控内容在表格软件中被作为公式执行。
func csvCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func (h *HTTP) getOrder(w http.ResponseWriter, r *http.Request) {
	o, err := h.orders.AdminGet(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "订单不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	lines := make([]map[string]any, 0, len(o.Lines))
	for _, l := range o.Lines {
		lines = append(lines, map[string]any{
			"title": l.Title, "spec": l.Spec, "sku": l.SKU,
			"unit_cents": l.UnitCents, "quantity": l.Quantity, "line_cents": l.LineCents,
		})
	}
	refunds := make([]map[string]any, 0, len(o.Refunds))
	for _, rf := range o.Refunds {
		providerRefundID := rf.ProviderRefundID
		if isDemoRequest(r) {
			providerRefundID = "已隐藏"
		}
		refunds = append(refunds, map[string]any{
			"provider": rf.Provider, "provider_refund_id": providerRefundID,
			"amount_cents": rf.AmountCents, "created_at": rf.CreatedAt,
		})
	}
	email, shipName, shipPhone, shipAddress := o.Email, o.ShipName, o.ShipPhone, o.ShipAddress
	trackingNumber, trackingURL := o.TrackingNumber, o.TrackingURL
	if isDemoRequest(r) {
		email, shipName, shipPhone, shipAddress = "已隐藏（公开演示）", "已隐藏", "已隐藏", "已隐藏"
		trackingNumber, trackingURL = "已隐藏", ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"public_id": o.PublicID, "status": o.Status, "email": email,
		"ship_name": shipName, "ship_phone": shipPhone, "ship_address": shipAddress, "ship_country": o.ShipCountry,
		"currency": o.Currency, "subtotal_cents": o.SubtotalCents, "total_cents": o.TotalCents,
		"shipping_cents": o.ShippingCents, "shipping_rule_name": o.ShippingRuleName, "tracking_carrier": o.TrackingCarrier, "tracking_number": trackingNumber, "tracking_url": trackingURL,
		"payment_provider": o.PaymentProvider, "created_at": o.CreatedAt,
		"lines": lines, "refunds": refunds,
	})
}

func (h *HTTP) refundOrder(w http.ResponseWriter, r *http.Request) {
	if h.pay == nil {
		writeErr(w, http.StatusInternalServerError, "支付未装配")
		return
	}
	err := h.pay.Refund(r.Context(), r.PathValue("id"))
	switch {
	case err == nil:
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.refund", "order", r.PathValue("id"))
		o, _ := h.orders.AdminGet(r.Context(), r.PathValue("id"))
		status := "refunded"
		if o != nil {
			status = o.Status
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, http.StatusNotFound, "订单不存在")
	case errors.Is(err, payment.ErrNotRefundable):
		writeErr(w, http.StatusConflict, "订单当前状态不可退款（仅已付订单可退）")
	case errors.Is(err, payment.ErrLocked):
		writeErr(w, http.StatusServiceUnavailable, "收款密钥未解锁，请重新登录后重试")
	default:
		// 退款失败（如网关拒绝）：admin 场景下回传原因便于排查。
		writeErr(w, http.StatusBadGateway, "退款失败："+err.Error())
	}
}

func (h *HTTP) fulfillOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Carrier        string `json:"carrier"`
		TrackingNumber string `json:"tracking_number"`
		TrackingURL    string `json:"tracking_url"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.orders.Fulfill(r.Context(), r.PathValue("id"), req.Carrier, req.TrackingNumber, req.TrackingURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 409, "订单当前状态不可发货（仅已付款订单可发货）")
		} else {
			writeErr(w, 400, "发货信息无效")
		}
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "order.fulfill", "order", r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": true, "status": "fulfilled"})
}
