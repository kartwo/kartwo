// 支付 Webhook HTTP / Payment Webhook Endpoint
// 功能：接收 Stripe Webhook，读【原始字节】交编排服务双校验+幂等；锁定/伪造一律非 2xx，绝不放行
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"errors"
	"io"
	"net/http"
)

// maxWebhookBody Webhook 体上限（防滥用）。
const maxWebhookBody = 1 << 20

// HTTP 承载支付公开端点（无鉴权、无 CSRF——靠 HMAC 签名鉴权）。
type HTTP struct {
	svc *Service
}

// NewHTTP 构造支付 HTTP 层。
func NewHTTP(svc *Service) *HTTP { return &HTTP{svc: svc} }

// Register 注册 Webhook 路由。务必不挂任何会预读/改写 body 的中间件，否则验签必败。
func (h *HTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/stripe", h.stripeWebhook)
	mux.HandleFunc("POST /webhooks/paypal", h.paypalWebhook)
}

func (h *HTTP) paypalWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	err = h.svc.HandlePayPalWebhook(r.Context(), body, r.Header)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	case errors.Is(err, ErrLocked):
		// 未配 webhook_id：返回非 2xx 交 PayPal 重投。
		http.Error(w, "paypal webhook not configured", http.StatusServiceUnavailable)
	case errors.Is(err, ErrBadSignature):
		http.Error(w, "invalid signature", http.StatusBadRequest)
	case errors.Is(err, ErrMismatch):
		http.Error(w, "order mismatch", http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *HTTP) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	// 读原始字节（验签必须基于未改写的字节）。
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("Stripe-Signature")

	err = h.svc.HandleWebhook(r.Context(), body, sig)
	switch {
	case err == nil:
		// 已处理 / 已去重 / 安全忽略：确认收到。
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	case errors.Is(err, ErrLocked):
		// 密钥锁定：返回非 2xx 交 Stripe 重投；绝不 200、绝不存未验签 payload。
		http.Error(w, "payment keys locked", http.StatusServiceUnavailable)
	case errors.Is(err, ErrBadSignature):
		// 验签失败（伪造/篡改/过期）。
		http.Error(w, "invalid signature", http.StatusBadRequest)
	case errors.Is(err, ErrMismatch):
		// 订单/金额对不上：拒绝改单。
		http.Error(w, "order mismatch", http.StatusBadRequest)
	default:
		// 未预期错误：返回 5xx 交网关重投。
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
