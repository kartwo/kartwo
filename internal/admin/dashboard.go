// 概览 HTTP / Dashboard Handler
// 功能：登录后首页概览——聚合订单(今日/近7日数与销售额、待处理)、商品数、库存告警、开店进度信号（需鉴权，只读）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-07 18:47:01
package admin

import "net/http"

// dashboard 返回概览指标。全部走 SQL 聚合、只读无事务、单连接顺序执行（防 N+1 与单连接死锁）。
// 开店进度（setup）三信号呼应北极星：有商品 / 已配收款 / 已配域名；三者齐即 ready。
func (h *HTTP) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.orders.DashboardStats(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	productCount, err := h.cat.CountActiveProducts(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	alerts, err := h.cat.StockAlerts(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}

	// 开店进度信号（复用既有能力，无新增存储）。
	pst, _ := h.svc.PaymentStatus()
	paymentConfigured := pst.StripeHasSecret || pst.PayPalHasSecret
	_, domainSource := h.effectiveDomain(ctx)
	domainConfigured := domainSource != "none"
	hasProducts := productCount > 0
	ready := hasProducts && paymentConfigured && domainConfigured

	writeJSON(w, http.StatusOK, map[string]any{
		"currency": stats.Currency,
		"orders": map[string]any{
			"today":               map[string]any{"count": stats.TodayCount, "sales_cents": stats.TodaySalesCents},
			"week":                map[string]any{"count": stats.WeekCount, "sales_cents": stats.WeekSalesCents},
			"pending_fulfillment": stats.PendingFulfillment,
		},
		"products": map[string]any{
			"count":      productCount,
			"zero_stock": alerts.ZeroStock,
			"low_stock":  alerts.LowStock,
		},
		"setup": map[string]any{
			"has_products":       hasProducts,
			"payment_configured": paymentConfigured,
			"domain_configured":  domainConfigured,
			"ready":              ready,
		},
	})
}
