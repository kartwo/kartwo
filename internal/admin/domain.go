// 域名设置 HTTP / Domain Settings Handlers
// 功能：向导域名步骤——读/写站点域名（写 settings.domain）、来源解析(env 覆盖 DB)展示、env 覆盖时只读、独立域名校验、向导步骤状态/跳过（需鉴权+CSRF）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-07 15:37:37
package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// keyWizardDomainSkipped 标记商家在开店向导里跳过了域名配置（先 HTTP 评估、稍后再配）。
const keyWizardDomainSkipped = "wizard.domain_skipped"

// effectiveDomain 按"env 覆盖 DB"口径解析当前生效域名及来源，仅用于后台展示/只读判定。
// 运行时权威解析在 server.EffectiveDomain（决定是否签发 HTTPS）；admin 不能 import server（会成环），
// 故此处按同口径复刻——两处口径必须保持一致：env 非空即用(来源 env)、否则读 DB(来源 db)、皆空 none。
func (h *HTTP) effectiveDomain(ctx context.Context) (domain, source string) {
	if env := strings.TrimSpace(h.envDomain); env != "" {
		return env, "env"
	}
	if d, err := h.settings.Domain(ctx); err == nil {
		if d = strings.TrimSpace(d); d != "" {
			return d, "db"
		}
	}
	return "", "none"
}

// getDomain 返回当前生效域名、来源、是否只读（env 覆盖）、本实例能否签发 HTTPS（仅 prod）。
func (h *HTTP) getDomain(w http.ResponseWriter, r *http.Request) {
	domain, source := h.effectiveDomain(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"domain":        domain,
		"source":        source,
		"readonly":      source == "env",
		"https_capable": h.secure, // prod=true；dev 填域名只写库、绝不签发 HTTPS（本地永远纯 HTTP）
	})
}

// setDomain 写入站点域名（向导域名步骤）。env 覆盖时只读拒写；域名后端独立校验（不依赖前端）。
// 域名改动需重启进程后由 autocert 签发/续期证书（决策：域名热切换不做，见 DECISIONS）。
func (h *HTTP) setDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if _, source := h.effectiveDomain(r.Context()); source == "env" {
		writeErr(w, http.StatusConflict, "域名由环境变量 KARTWO_DOMAIN 提供（只读）。请改环境变量后重启，或清空 KARTWO_DOMAIN 改用后台配置。")
		return
	}
	domain, err := validateDomain(req.Domain)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.settings.SetDomain(r.Context(), domain); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return
	}
	h.getDomain(w, r)
}

// validateDomain 独立校验域名（后端自守，不依赖前端）：
// 拒空/纯空白、拒协议前缀(http:// https://)、拒路径(/)、拒空格与非法字符、需含点(FQDN，否则无法签发公网证书)。
func validateDomain(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if d == "" {
		return "", errors.New("域名不能为空")
	}
	if l := strings.ToLower(d); strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
		return "", errors.New("请只填域名本身，不要带 http:// 或 https://")
	}
	if strings.Contains(d, "/") {
		return "", errors.New("请只填域名，不要带路径（如 shop.example.com，而不是 shop.example.com/store）")
	}
	if strings.ContainsAny(d, " \t\r\n") {
		return "", errors.New("域名不能包含空格")
	}
	for _, c := range d {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-':
			// 合法字符
		default:
			return "", errors.New("域名含非法字符（只允许字母、数字、点、连字符）")
		}
	}
	if !strings.Contains(d, ".") {
		return "", errors.New("请填写完整域名（如 shop.example.com）")
	}
	return d, nil
}

// wizardDomainStatus 报告开店向导是否仍需展示「配置域名」步骤。
// needed = 未配域名(env/db 皆无) 且 未跳过；已配(含 env 提供)或跳过后不再打扰。
func (h *HTTP) wizardDomainStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, source := h.effectiveDomain(ctx)
	configured := source == "db" || source == "env"
	skipped := h.settingExists(ctx, keyWizardDomainSkipped)
	writeJSON(w, http.StatusOK, map[string]any{"needed": !configured && !skipped, "configured": configured})
}

// wizardDomainSkip 记录「先 HTTP 评估、稍后再配」，使向导不再展示域名步骤（商家随时可从后台域名页配置）。
// 跳过不造任何新机器：prod 域名空即原生 HTTP-only 评估态(:80 纯 HTTP、不发 HSTS)。
func (h *HTTP) wizardDomainSkip(w http.ResponseWriter, r *http.Request) {
	if err := h.settings.SetPlain(r.Context(), keyWizardDomainSkipped, "1"); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
