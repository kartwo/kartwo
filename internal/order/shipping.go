// 配送履约服务 / Shipping Fulfillment Service
// 功能：显式配送范围与运费规则的一致快照；全球兜底不会隐含开通目的地
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-07 13:00:00
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var ErrNoShippingRule = errors.New("order: 该国家暂无配送方式")

const ShippingCountriesSetting = "shipping.enabled_countries"

type shippingRate struct {
	name, countries string
	rate, freeOver  int64
}
type shippingState struct {
	enabled map[string]bool
	rates   []shippingRate
}

// loadShipping uses the caller's transaction, so country enablement and rates
// cannot come from different moments during quote or checkout.
func loadShipping(ctx context.Context, db sqlcgen.DBTX) (shippingState, error) {
	state := shippingState{enabled: map[string]bool{}}
	setting, err := sqlcgen.New(db).GetSetting(ctx, ShippingCountriesSetting)
	configured := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	if configured {
		if setting.Encrypted != 0 {
			return state, fmt.Errorf("shipping: invalid country configuration")
		}
		for _, code := range strings.Split(setting.Value, ",") {
			code = strings.ToUpper(strings.TrimSpace(code))
			if knownCountry(code) {
				state.enabled[code] = true
			}
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT name,countries,rate_cents,free_over_cents FROM shipping_zone WHERE active=1 AND deleted_at IS NULL ORDER BY CASE WHEN countries='' THEN 1 ELSE 0 END,id")
	if err != nil {
		return state, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rate shippingRate
		if err = rows.Scan(&rate.name, &rate.countries, &rate.rate, &rate.freeOver); err != nil {
			return state, err
		}
		state.rates = append(state.rates, rate)
		if !configured {
			for _, code := range strings.Split(rate.countries, ",") {
				code = strings.ToUpper(strings.TrimSpace(code))
				if knownCountry(code) {
					state.enabled[code] = true
				}
			}
		}
	}
	return state, rows.Err()
}

func (state shippingState) forCountry(country string, subtotal int64) (int64, string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if !state.enabled[country] {
		return 0, "", ErrNoShippingRule
	}
	for _, rate := range state.rates {
		matches := strings.TrimSpace(rate.countries) == ""
		for _, code := range strings.Split(rate.countries, ",") {
			if strings.ToUpper(strings.TrimSpace(code)) == country {
				matches = true
			}
		}
		if matches {
			if rate.rate < 0 || rate.freeOver < 0 {
				return 0, "", fmt.Errorf("shipping: invalid stored rate")
			}
			if rate.freeOver > 0 && subtotal >= rate.freeOver {
				return 0, rate.name, nil
			}
			return rate.rate, rate.name, nil
		}
	}
	return 0, "", ErrNoShippingRule
}

// ShippingCountries preserves explicit settings and legacy explicit zone
// destinations. A global fallback never expands the checkout destination list.
func (s *Service) ShippingCountries(ctx context.Context) ([]Country, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := loadShipping(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return countriesFromSet(state.enabled), nil
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
