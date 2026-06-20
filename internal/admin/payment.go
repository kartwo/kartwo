// 收款设置 HTTP / Payment Settings Handlers
// 功能：后台收款页——读/存 Stripe 密钥（pk 明文、sk/whsec 加密存）；存后立即重载内存缓存使其生效（需鉴权+CSRF）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/settings"
)

// getPayment 返回收款配置状态（绝不回传 sk/whsec 明文，仅报是否已配置）。
func (h *HTTP) getPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mode, err := h.settings.Get(ctx, payment.KeyStripeMode)
	if errors.Is(err, settings.ErrNotFound) || strings.TrimSpace(mode) == "" {
		mode = "test"
	}
	pub, _ := h.settings.Get(ctx, payment.KeyStripePublishable)
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":        mode,
		"publishable": pub,
		"has_secret":  h.settingExists(ctx, payment.KeyStripeSecret),
		"has_webhook": h.settingExists(ctx, payment.KeyStripeWebhookSecret),
	})
}

// setPayment 保存收款配置。空的 secret/webhook 表示「保持原值」，便于改 mode/pk 时不必重输密钥。
func (h *HTTP) setPayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode          string `json:"mode"`
		Publishable   string `json:"publishable"`
		Secret        string `json:"secret"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	mode := strings.TrimSpace(req.Mode)
	if mode != "test" && mode != "live" {
		writeErr(w, http.StatusBadRequest, "mode 只能是 test 或 live")
		return
	}

	// 取本会话已解锁的 KEK（加密存密钥用）。
	ac := authFrom(r.Context())
	kek, ok := h.svc.Key(ac.SessionToken)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "会话密钥不可用，请重新登录")
		return
	}

	ctx := r.Context()
	if err := h.settings.SetPlain(ctx, payment.KeyStripeMode, mode); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return
	}
	if err := h.settings.SetPlain(ctx, payment.KeyStripePublishable, strings.TrimSpace(req.Publishable)); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return
	}
	if s := strings.TrimSpace(req.Secret); s != "" {
		if err := h.settings.SetEncrypted(ctx, payment.KeyStripeSecret, []byte(s), kek); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
	}
	if s := strings.TrimSpace(req.WebhookSecret); s != "" {
		if err := h.settings.SetEncrypted(ctx, payment.KeyStripeWebhookSecret, []byte(s), kek); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
	}

	// 立即重载内存缓存，使改动即时生效（含 Webhook 验签密钥）。
	if err := h.svc.ReloadKeys(ctx, kek); err != nil {
		writeErr(w, http.StatusInternalServerError, "重载收款密钥失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"mode":        mode,
		"publishable": strings.TrimSpace(req.Publishable),
		"has_secret":  h.settingExists(ctx, payment.KeyStripeSecret),
		"has_webhook": h.settingExists(ctx, payment.KeyStripeWebhookSecret),
	})
}

// settingExists 报告某设置项是否已存在（不解密、不暴露值）。
func (h *HTTP) settingExists(ctx context.Context, key string) bool {
	_, err := h.settings.Get(ctx, key)
	return err == nil
}
