// SMTP 设置 HTTP / SMTP Settings Handlers
// 功能：后台 SMTP 设置（password 加密存/其余明文；env 覆盖只读）+ 测试发信 + 向导邮件步骤状态/跳过（需鉴权+CSRF）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kartwo/kartwo/internal/mail"
)

// keyWizardSMTPSkipped 标记商家在开店向导里跳过了 SMTP 配置（稍后再配）。
const keyWizardSMTPSkipped = "wizard.smtp_skipped"

// getSMTP 返回 SMTP 配置状态（绝不回传密码明文，仅报 has_password）。env 覆盖时 readonly=true。
func (h *HTTP) getSMTP(w http.ResponseWriter, r *http.Request) {
	if h.mailCache == nil {
		writeErr(w, http.StatusInternalServerError, "邮件未装配")
		return
	}
	st := h.mailCache.Status(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"source": st.Source, "readonly": st.Source == "env",
		"host": st.Host, "port": st.Port, "username": st.Username,
		"from_address": st.FromAddress, "from_name": st.FromName,
		"encryption": st.Encryption, "has_password": st.HasPassword, "configured": st.Configured,
	})
}

// setSMTP 保存 SMTP 配置。password 空=保持原值（同支付纪律）；env 覆盖时只读拒写 409。
func (h *HTTP) setSMTP(w http.ResponseWriter, r *http.Request) {
	if h.mailCache == nil {
		writeErr(w, http.StatusInternalServerError, "邮件未装配")
		return
	}
	if h.mailCache.EnvOverride() {
		writeErr(w, http.StatusConflict, "SMTP 由环境变量提供（只读）。请改环境变量后重启，或清空 SMTP_* 改用后台配置。")
		return
	}
	var req struct {
		Host        string `json:"host"`
		Port        string `json:"port"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		FromAddress string `json:"from_address"`
		FromName    string `json:"from_name"`
		Encryption  string `json:"encryption"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	host := strings.TrimSpace(req.Host)
	from := strings.TrimSpace(req.FromAddress)
	enc := strings.TrimSpace(req.Encryption)
	if host == "" || from == "" {
		writeErr(w, http.StatusBadRequest, "SMTP 主机与发件地址必填")
		return
	}
	if !strings.Contains(from, "@") {
		writeErr(w, http.StatusBadRequest, "发件地址需为合法邮箱")
		return
	}
	if p, err := strconv.Atoi(strings.TrimSpace(req.Port)); err != nil || p < 1 || p > 65535 {
		writeErr(w, http.StatusBadRequest, "端口需为 1–65535 的数字")
		return
	}
	if enc != "none" && enc != "starttls" && enc != "tls" {
		writeErr(w, http.StatusBadRequest, "加密方式只能是 none / starttls / tls")
		return
	}

	ac := authFrom(r.Context())
	kek, ok := h.svc.Key(ac.SessionToken)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "会话密钥不可用，请重新登录")
		return
	}
	ctx := r.Context()
	if !h.saveSetting(ctx, w, mail.KeySMTPHost, host, false, kek) ||
		!h.saveSetting(ctx, w, mail.KeySMTPPort, strings.TrimSpace(req.Port), false, kek) ||
		!h.saveSetting(ctx, w, mail.KeySMTPUsername, strings.TrimSpace(req.Username), false, kek) ||
		!h.saveSetting(ctx, w, mail.KeySMTPFromAddress, from, false, kek) ||
		!h.saveSetting(ctx, w, mail.KeySMTPFromName, strings.TrimSpace(req.FromName), false, kek) ||
		!h.saveSetting(ctx, w, mail.KeySMTPEncryption, enc, false, kek) ||
		!h.saveSetting(ctx, w, mail.KeySMTPPassword, req.Password, true, kek) { // 空则保持原值
		return
	}
	// 立即重载缓存，使 worker/测试发信即时生效。
	if err := h.svc.ReloadMailKeys(ctx, kek); err != nil {
		writeErr(w, http.StatusInternalServerError, "重载 SMTP 配置失败")
		return
	}
	h.getSMTP(w, r)
}

// smtpTest 用当前（已保存并解锁的）SMTP 配置同步发一封测试邮件到指定地址。
func (h *HTTP) smtpTest(w http.ResponseWriter, r *http.Request) {
	if h.mailCache == nil {
		writeErr(w, http.StatusInternalServerError, "邮件未装配")
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	to := strings.TrimSpace(req.To)
	if !strings.Contains(to, "@") {
		writeErr(w, http.StatusBadRequest, "请填写合法的收件邮箱")
		return
	}
	cfg, ok := h.mailCache.Config()
	if !ok {
		writeErr(w, http.StatusBadRequest, "SMTP 未配置或未解锁——请先保存 SMTP 配置")
		return
	}
	if err := mail.Send(cfg, to, "Kartwo SMTP 测试邮件", "这是一封来自 Kartwo 的测试邮件。收到即说明 SMTP 配置可用。\n\nThis is a test email from Kartwo. If you received it, your SMTP settings work."); err != nil {
		// 不回传底层错误细节里可能含的主机信息足够诊断，但不含密码；直接回 err.Error()（不含凭证）。
		writeErr(w, http.StatusBadGateway, "发送失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// wizardSMTPStatus 报告开店向导是否仍需展示「配置邮件」步骤。needed=未配置且未跳过。
func (h *HTTP) wizardSMTPStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	configured := h.mailCache != nil && h.mailCache.Status(ctx).Configured
	skipped := h.settingExists(ctx, keyWizardSMTPSkipped)
	writeJSON(w, http.StatusOK, map[string]any{"needed": !configured && !skipped, "configured": configured})
}

// wizardSMTPSkip 记录「稍后再配」，使向导不再展示邮件步骤（商家随时可从 SMTP 设置页配置）。
func (h *HTTP) wizardSMTPSkip(w http.ResponseWriter, r *http.Request) {
	if err := h.settings.SetPlain(r.Context(), keyWizardSMTPSkipped, "1"); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
