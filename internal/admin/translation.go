// 翻译服务设置 / Translation Service Settings
// 功能：加密保存 DeepL API Key，按商家点击将中文辅助文本翻译为英文
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-02 18:10:00
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	translationPlanKey = "translation.deepl.plan"
	translationKeyKey  = "translation.deepl.api_key"
)

func (h *HTTP) getTranslationSettings(w http.ResponseWriter, r *http.Request) {
	plan, _ := h.settings.Get(r.Context(), translationPlanKey)
	if plan == "" {
		plan = "developer"
	}
	_, err := h.settings.Get(r.Context(), translationKeyKey)
	writeJSON(w, http.StatusOK, map[string]any{"provider": "deepl", "plan": plan, "has_api_key": err == nil, "configured": err == nil,
		"developer_quota": "累计 100 万字符免费", "growth_quota": "月付含 100 万字符；年付含 1200 万字符", "price_note": "价格及超额费用以 DeepL 账户所在地区的结算页为准。"})
}

func (h *HTTP) setTranslationSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Plan   string `json:"plan"`
		APIKey string `json:"api_key"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	plan := strings.TrimSpace(req.Plan)
	if plan != "developer" && plan != "growth" {
		writeErr(w, http.StatusBadRequest, "套餐只能是 developer 或 growth")
		return
	}
	ac := authFrom(r.Context())
	kek, ok := h.svc.Key(ac.SessionToken)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "会话密钥不可用，请重新登录")
		return
	}
	if err := h.settings.SetPlain(r.Context(), translationPlanKey, plan); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败")
		return
	}
	if strings.TrimSpace(req.APIKey) != "" {
		if err := h.settings.SetEncrypted(r.Context(), translationKeyKey, []byte(strings.TrimSpace(req.APIKey)), kek); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
	}
	h.recordAudit(r, ac.AdminID, "translation.settings_update", "settings", "translation")
	h.getTranslationSettings(w, r)
}

func (h *HTTP) translateText(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" || len([]rune(text)) > 2000 {
		writeErr(w, http.StatusBadRequest, "翻译内容长度应为 1–2000 字")
		return
	}
	if _, err := h.settings.Get(r.Context(), translationKeyKey); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "尚未设置翻译 API，请先前往翻译服务设置")
		return
	}
	ac := authFrom(r.Context())
	kek, ok := h.svc.Key(ac.SessionToken)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "会话密钥不可用，请重新登录")
		return
	}
	key, err := h.settings.GetEncrypted(r.Context(), translationKeyKey, kek)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "翻译服务未配置")
		return
	}
	plan, _ := h.settings.Get(r.Context(), translationPlanKey)
	translated, err := deepLTranslate(r.Context(), plan, string(key), text)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "英文翻译暂不可用，请稍后重试或手工填写")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": translated})
}

func deepLTranslate(ctx context.Context, plan, key, text string) (string, error) {
	endpoint := "https://api-free.deepl.com/v2/translate"
	if plan == "growth" {
		endpoint = "https://api.deepl.com/v2/translate"
	}
	body, _ := json.Marshal(map[string]any{"text": []string{text}, "source_lang": "ZH", "target_lang": "EN"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("deepl status %d", resp.StatusCode)
	}
	var out struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if json.Unmarshal(raw, &out) != nil || len(out.Translations) == 0 || strings.TrimSpace(out.Translations[0].Text) == "" {
		return "", fmt.Errorf("deepl invalid response")
	}
	return out.Translations[0].Text, nil
}
