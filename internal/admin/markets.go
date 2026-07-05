// 市场选择 HTTP / Market Selection Handlers
// 功能：向导「选择主攻市场」——列市场、读/设当前市场（需鉴权+CSRF）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-19 21:22:05
package admin

import (
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/market"
	"github.com/kartwo/kartwo/internal/settings"
)

func marketJSON(m market.Market) map[string]any {
	return map[string]any{
		"code": m.Code, "name": m.Name, "currency": m.Currency, "locale": m.Locale,
		"rtl": m.RTL, "providers": m.Providers, "status": string(m.Status),
		"available": m.Status == market.Available, "enables": m.Enables, "note": m.Note,
	}
}

func (h *HTTP) listMarkets(w http.ResponseWriter, _ *http.Request) {
	ms := market.List()
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, marketJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"markets": out})
}

func (h *HTTP) getMarket(w http.ResponseWriter, r *http.Request) {
	_, err := h.settings.Get(r.Context(), "market.code")
	configured := err == nil
	m := h.settings.Market(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"configured": configured, "current": marketJSON(m)})
}

func (h *HTTP) setMarket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.settings.SetMarketCode(r.Context(), req.Code); err != nil {
		if errors.Is(err, settings.ErrMarketUnavailable) {
			writeErr(w, http.StatusConflict, "该市场即将上线，暂不可选")
			return
		}
		writeErr(w, http.StatusBadRequest, "无效的市场")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "current": marketJSON(h.settings.Market(r.Context()))})
}
