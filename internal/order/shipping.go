// 配送履约服务 / Shipping Fulfillment Service
// 功能：按国家匹配固定运费，锁定订单配送快照并安全记录发货追踪
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-07 13:00:00
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kartwo/kartwo/internal/settings"
)

var ErrNoShippingRule = errors.New("order: 该国家暂无配送方式")

// ShippingCountriesSetting 保存商家明确启用的配送国家代码列表。
const ShippingCountriesSetting = "shipping.enabled_countries"

// ShippingCountries 返回管理员在配送分区中明确勾选的 ISO 国家代码。
// 全球兜底仅参与运费匹配，不会把未勾选地区暴露到结账页。
func (s *Service) ShippingCountries(ctx context.Context) ([]Country, error) {
	codes, configured, err := s.configuredShippingCountries(ctx)
	if err != nil {
		return nil, err
	}
	if configured {
		return countriesFromSet(codes), nil
	}
	// 兼容首次升级：尚未保存独立配送范围时，从旧分区显式国家推导，不把 Global 展开。
	rows, err := s.db.QueryContext(ctx, "SELECT countries FROM shipping_zone WHERE active=1 AND deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]bool{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, c := range strings.Split(raw, ",") {
			c = strings.ToUpper(strings.TrimSpace(c))
			if knownCountry(c) && !seen[c] {
				seen[c] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return countriesFromSet(seen), nil
}

func countriesFromSet(seen map[string]bool) []Country {
	out := make([]Country, 0, len(seen))
	for _, country := range countryOptions {
		if seen[country.Code] {
			out = append(out, Country{Code: country.Code, Name: country.Name, Continent: continentFor(country.Code)})
		}
	}
	return out
}

func (s *Service) configuredShippingCountries(ctx context.Context) (map[string]bool, bool, error) {
	raw, err := s.settings.Get(ctx, ShippingCountriesSetting)
	if errors.Is(err, settings.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	seen := map[string]bool{}
	for _, code := range strings.Split(raw, ",") {
		code = strings.ToUpper(strings.TrimSpace(code))
		if knownCountry(code) {
			seen[code] = true
		}
	}
	return seen, true, nil
}

func (s *Service) shippingFor(ctx context.Context, country string, subtotal int64) (int64, string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if enabled, configured, err := s.configuredShippingCountries(ctx); err != nil {
		return 0, "", err
	} else if configured && !enabled[country] {
		return 0, "", ErrNoShippingRule
	}
	rows, err := s.db.QueryContext(ctx, "SELECT name,countries,rate_cents,free_over_cents FROM shipping_zone WHERE active=1 AND deleted_at IS NULL ORDER BY CASE WHEN countries='' THEN 1 ELSE 0 END,id")
	if err != nil {
		return 0, "", fmt.Errorf("shipping: 读取规则: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rate, free int64
		var name, countries string
		if err := rows.Scan(&name, &countries, &rate, &free); err != nil {
			return 0, "", err
		}
		ok := strings.TrimSpace(countries) == ""
		for _, c := range strings.Split(countries, ",") {
			if strings.TrimSpace(c) == country {
				ok = true
			}
		}
		if ok {
			if free > 0 && subtotal >= free {
				return 0, name, nil
			}
			return rate, name, nil
		}
	}
	return 0, "", ErrNoShippingRule
}
func (s *Service) Fulfill(ctx context.Context, id, carrier, number, link string) error {
	if strings.TrimSpace(carrier) == "" || strings.TrimSpace(number) == "" || len(number) > 128 {
		return ErrInvalidInfo
	}
	if link != "" {
		u, e := url.Parse(link)
		if e != nil || u.Scheme != "https" || u.Host == "" {
			return ErrInvalidInfo
		}
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	r, e := tx.ExecContext(ctx, "UPDATE \"order\" SET status='fulfilled',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND status='paid'", id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO shipment(order_id,carrier,tracking_number,tracking_url) SELECT id,?,?,? FROM \"order\" WHERE public_id=?", carrier, number, link, id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
