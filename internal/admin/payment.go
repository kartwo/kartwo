// 收款设置 HTTP / Payment Settings Handlers
// 功能：后台收款页——读/存 Stripe + PayPal 密钥（pk/client_id 明文、sk/whsec/secret 加密存）；存后立即重载缓存（需鉴权+CSRF）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-23 00:20:52
package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/settings"
)

// getPayment 返回 Stripe + PayPal 收款配置状态（绝不回传密钥明文，仅报是否已配置）。
// 某通道 env 覆盖激活时该块 source=env、readonly=true（收款页置灰）。
func (h *HTTP) getPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := h.svc.PaymentStatus()

	stripe := map[string]any{"source": "db", "readonly": false}
	if st.StripeSource == "env" {
		stripe = map[string]any{"source": "env", "readonly": true, "mode": st.StripeMode,
			"publishable": st.StripePublishable, "has_secret": st.StripeHasSecret, "has_webhook": st.StripeHasWebhook}
	} else {
		mode, err := h.settings.Get(ctx, payment.KeyStripeMode)
		if errors.Is(err, settings.ErrNotFound) || strings.TrimSpace(mode) == "" {
			mode = "test"
		}
		pub, _ := h.settings.Get(ctx, payment.KeyStripePublishable)
		stripe["mode"], stripe["publishable"] = mode, pub
		stripe["has_secret"] = h.settingExists(ctx, payment.KeyStripeSecret)
		stripe["has_webhook"] = h.settingExists(ctx, payment.KeyStripeWebhookSecret)
	}

	paypal := map[string]any{"source": "db", "readonly": false}
	if st.PayPalSource == "env" {
		paypal = map[string]any{"source": "env", "readonly": true, "mode": st.PayPalMode,
			"client_id": st.PayPalClientID, "has_secret": st.PayPalHasSecret, "webhook_id": st.PayPalWebhookID}
	} else {
		mode, err := h.settings.Get(ctx, payment.KeyPayPalMode)
		if errors.Is(err, settings.ErrNotFound) || strings.TrimSpace(mode) == "" {
			mode = "sandbox"
		}
		cid, _ := h.settings.Get(ctx, payment.KeyPayPalClientID)
		wid, _ := h.settings.Get(ctx, payment.KeyPayPalWebhookID)
		paypal["mode"], paypal["client_id"], paypal["webhook_id"] = mode, cid, wid
		paypal["has_secret"] = h.settingExists(ctx, payment.KeyPayPalSecret)
	}

	writeJSON(w, http.StatusOK, map[string]any{"stripe": stripe, "paypal": paypal})
}

// setPayment 保存收款配置。body 含可选 stripe/paypal 两块；空的密钥字段表示「保持原值」。
func (h *HTTP) setPayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stripe *struct {
			Mode          string `json:"mode"`
			Publishable   string `json:"publishable"`
			Secret        string `json:"secret"`
			WebhookSecret string `json:"webhook_secret"`
		} `json:"stripe"`
		PayPal *struct {
			Mode      string `json:"mode"`
			ClientID  string `json:"client_id"`
			Secret    string `json:"secret"`
			WebhookID string `json:"webhook_id"`
		} `json:"paypal"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	ac := authFrom(r.Context())
	kek, ok := h.svc.Key(ac.SessionToken)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "会话密钥不可用，请重新登录")
		return
	}
	st, _ := h.svc.PaymentStatus()
	ctx := r.Context()

	if req.Stripe != nil {
		if st.StripeSource == "env" {
			writeErr(w, http.StatusConflict, "Stripe 密钥由环境变量提供（只读）。请改环境变量后重启，或清空 STRIPE_* 改用后台配置。")
			return
		}
		mode := strings.TrimSpace(req.Stripe.Mode)
		if mode != "test" && mode != "live" {
			writeErr(w, http.StatusBadRequest, "Stripe mode 只能是 test 或 live")
			return
		}
		if !h.saveSetting(ctx, w, payment.KeyStripeMode, mode, false, kek) ||
			!h.saveSetting(ctx, w, payment.KeyStripePublishable, strings.TrimSpace(req.Stripe.Publishable), false, kek) ||
			!h.saveSetting(ctx, w, payment.KeyStripeSecret, strings.TrimSpace(req.Stripe.Secret), true, kek) ||
			!h.saveSetting(ctx, w, payment.KeyStripeWebhookSecret, strings.TrimSpace(req.Stripe.WebhookSecret), true, kek) {
			return
		}
	}

	if req.PayPal != nil {
		if st.PayPalSource == "env" {
			writeErr(w, http.StatusConflict, "PayPal 密钥由环境变量提供（只读）。请改环境变量后重启，或清空 PAYPAL_* 改用后台配置。")
			return
		}
		mode := strings.TrimSpace(req.PayPal.Mode)
		if mode != "sandbox" && mode != "live" {
			writeErr(w, http.StatusBadRequest, "PayPal mode 只能是 sandbox 或 live")
			return
		}
		if !h.saveSetting(ctx, w, payment.KeyPayPalMode, mode, false, kek) ||
			!h.saveSetting(ctx, w, payment.KeyPayPalClientID, strings.TrimSpace(req.PayPal.ClientID), false, kek) ||
			!h.saveSetting(ctx, w, payment.KeyPayPalWebhookID, strings.TrimSpace(req.PayPal.WebhookID), false, kek) ||
			!h.saveSetting(ctx, w, payment.KeyPayPalSecret, strings.TrimSpace(req.PayPal.Secret), true, kek) {
			return
		}
	}

	// 立即重载内存缓存，使改动即时生效。
	if err := h.svc.ReloadKeys(ctx, kek); err != nil {
		writeErr(w, http.StatusInternalServerError, "重载收款密钥失败")
		return
	}
	h.getPayment(w, r)
}

// saveSetting 写一个设置项；encrypted=true 且 value 空则跳过（保持原值）。失败已写响应、返回 false。
func (h *HTTP) saveSetting(ctx context.Context, w http.ResponseWriter, key, value string, encrypted bool, kek []byte) bool {
	if encrypted {
		if value == "" {
			return true // 留空=保持原值
		}
		if err := h.settings.SetEncrypted(ctx, key, []byte(value), kek); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return false
		}
		return true
	}
	if err := h.settings.SetPlain(ctx, key, value); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return false
	}
	return true
}

// settingExists 报告某设置项是否已存在（不解密、不暴露值）。
func (h *HTTP) settingExists(ctx context.Context, key string) bool {
	_, err := h.settings.Get(ctx, key)
	return err == nil
}
