// 配送设置 HTTP 接口 / Shipping Settings Handlers
// 功能：独立维护可配送范围、默认运费与特殊地区规则，不暴露内部主键
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-08 15:10:00
package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/settings"
)

type shippingZoneInput struct {
	Name      string `json:"name"`
	Countries string `json:"countries"`
	Rate      int64  `json:"rate_cents"`
	Free      int64  `json:"free_over_cents"`
}

func (h *HTTP) listShippingCountries(w http.ResponseWriter, r *http.Request) {
	enabled, err := h.orders.ShippingCountries(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取配送范围失败")
		return
	}
	codes := make([]string, 0, len(enabled))
	for _, country := range enabled {
		codes = append(codes, country.Code)
	}
	writeJSON(w, http.StatusOK, map[string]any{"countries": order.CountryOptions(), "enabled": codes})
}

func (h *HTTP) saveShippingCountries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Countries []string `json:"countries"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	normalized, ok := order.NormalizeShippingCountries(strings.Join(req.Countries, ","))
	if !ok {
		writeErr(w, http.StatusBadRequest, "包含不支持的国家/地区")
		return
	}
	if err := h.settings.SetPlain(r.Context(), order.ShippingCountriesSetting, normalized); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存配送范围失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shipping.countries_update", "settings", "shipping-countries")
	writeJSON(w, http.StatusOK, map[string]any{"countries": strings.Split(normalized, ",")})
}

func (h *HTTP) listShippingZones(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.db.QueryContext(r.Context(), "SELECT public_id,name,countries,rate_cents,free_over_cents,active FROM shipping_zone WHERE deleted_at IS NULL ORDER BY CASE WHEN countries='' THEN 0 ELSE 1 END,id")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, countries string
		var rate, free, active int64
		if err := rows.Scan(&id, &name, &countries, &rate, &free, &active); err != nil {
			writeErr(w, http.StatusInternalServerError, "内部错误")
			return
		}
		out = append(out, map[string]any{"public_id": id, "name": name, "countries": countries, "rate_cents": rate, "free_over_cents": free, "active": active == 1, "is_default": countries == ""})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zones": out})
}

func validateShippingZoneInput(input *shippingZoneInput, requireCountries bool) bool {
	input.Name = strings.TrimSpace(input.Name)
	input.Countries = strings.ToUpper(strings.TrimSpace(input.Countries))
	if input.Name == "" || input.Rate < 0 || input.Free < 0 || (requireCountries && input.Countries == "") {
		return false
	}
	normalized, ok := order.NormalizeShippingCountries(input.Countries)
	if !ok || (requireCountries && normalized == "") {
		return false
	}
	input.Countries = normalized
	return true
}

func (h *HTTP) createShippingZone(w http.ResponseWriter, r *http.Request) {
	var input shippingZoneInput
	if !readJSON(w, r, &input) {
		return
	}
	if !validateShippingZoneInput(&input, true) {
		writeErr(w, http.StatusBadRequest, "特殊规则必须填写名称、有效国家和非负金额")
		return
	}
	if enabled, err := h.shippingCountriesEnabled(r.Context(), input.Countries); err != nil {
		writeErr(w, http.StatusInternalServerError, "读取配送范围失败")
		return
	} else if !enabled {
		writeErr(w, http.StatusBadRequest, "特殊规则只能选择已启用的配送国家")
		return
	}
	if overlap, err := h.shippingCountriesOverlap(r.Context(), input.Countries, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "检查配送规则失败")
		return
	} else if overlap {
		writeErr(w, http.StatusConflict, "所选国家已属于另一条特殊规则")
		return
	}
	id := uuid.Must(uuid.NewV7()).String()
	if _, err := h.svc.db.ExecContext(r.Context(), "INSERT INTO shipping_zone(public_id,name,countries,rate_cents,free_over_cents) VALUES(?,?,?,?,?)", id, input.Name, input.Countries, input.Rate, input.Free); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存配送规则失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shipping_zone.create", "shipping_zone", id)
	writeJSON(w, http.StatusCreated, map[string]any{"public_id": id})
}

func (h *HTTP) saveDefaultShippingZone(w http.ResponseWriter, r *http.Request) {
	var input shippingZoneInput
	if !readJSON(w, r, &input) {
		return
	}
	input.Name, input.Countries = "Default", ""
	if !validateShippingZoneInput(&input, false) {
		writeErr(w, http.StatusBadRequest, "默认运费金额无效")
		return
	}
	var id string
	err := h.svc.db.QueryRowContext(r.Context(), "SELECT public_id FROM shipping_zone WHERE countries='' AND deleted_at IS NULL ORDER BY id LIMIT 1").Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = uuid.Must(uuid.NewV7()).String()
		_, err = h.svc.db.ExecContext(r.Context(), "INSERT INTO shipping_zone(public_id,name,countries,rate_cents,free_over_cents) VALUES(?,?,?,?,?)", id, input.Name, "", input.Rate, input.Free)
	case err == nil:
		_, err = h.svc.db.ExecContext(r.Context(), "UPDATE shipping_zone SET name=?,rate_cents=?,free_over_cents=?,active=1,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND deleted_at IS NULL", input.Name, input.Rate, input.Free, id)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "保存默认运费失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shipping.default_update", "shipping_zone", id)
	writeJSON(w, http.StatusOK, map[string]any{"public_id": id})
}

func (h *HTTP) updateShippingZone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input shippingZoneInput
	if !readJSON(w, r, &input) {
		return
	}
	if !validateShippingZoneInput(&input, true) {
		writeErr(w, http.StatusBadRequest, "特殊规则必须填写名称、有效国家和非负金额")
		return
	}
	if enabled, err := h.shippingCountriesEnabled(r.Context(), input.Countries); err != nil {
		writeErr(w, http.StatusInternalServerError, "读取配送范围失败")
		return
	} else if !enabled {
		writeErr(w, http.StatusBadRequest, "特殊规则只能选择已启用的配送国家")
		return
	}
	if overlap, err := h.shippingCountriesOverlap(r.Context(), input.Countries, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "检查配送规则失败")
		return
	} else if overlap {
		writeErr(w, http.StatusConflict, "所选国家已属于另一条特殊规则")
		return
	}
	result, err := h.svc.db.ExecContext(r.Context(), "UPDATE shipping_zone SET name=?,countries=?,rate_cents=?,free_over_cents=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND countries<>'' AND deleted_at IS NULL", input.Name, input.Countries, input.Rate, input.Free, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "更新配送规则失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeErr(w, http.StatusNotFound, "配送规则不存在")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shipping_zone.update", "shipping_zone", id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) deleteShippingZone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := h.svc.db.ExecContext(r.Context(), "UPDATE shipping_zone SET deleted_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND countries<>'' AND deleted_at IS NULL", id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "删除配送规则失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeErr(w, http.StatusNotFound, "配送规则不存在")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shipping_zone.delete", "shipping_zone", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) shippingCountriesOverlap(ctx context.Context, raw, excludeID string) (bool, error) {
	wanted := map[string]bool{}
	for _, code := range strings.Split(raw, ",") {
		wanted[code] = true
	}
	rows, err := h.svc.db.QueryContext(ctx, "SELECT public_id,countries FROM shipping_zone WHERE countries<>'' AND active=1 AND deleted_at IS NULL")
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, countries string
		if err := rows.Scan(&id, &countries); err != nil {
			return false, err
		}
		if id == excludeID {
			continue
		}
		for _, code := range strings.Split(countries, ",") {
			if wanted[code] {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

func (h *HTTP) shippingCountriesEnabled(ctx context.Context, raw string) (bool, error) {
	enabled, err := h.settings.Get(ctx, order.ShippingCountriesSetting)
	if errors.Is(err, settings.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	allowed := map[string]bool{}
	for _, code := range strings.Split(enabled, ",") {
		allowed[code] = true
	}
	for _, code := range strings.Split(raw, ",") {
		if !allowed[code] {
			return false, nil
		}
	}
	return true, nil
}
