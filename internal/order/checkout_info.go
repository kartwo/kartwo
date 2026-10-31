// 结算资料校验 / Checkout Information Validation
// 功能：规范多国地址并保留历史服务调用兼容；错误不回显客户资料
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 17:24:11
package order

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func checkoutText(value string, max int, multiline bool) bool {
	if !utf8.ValidString(value) || len(value) > max {
		return false
	}
	for _, ch := range value {
		if unicode.IsControl(ch) && (!multiline || ch != '\n') || ch == '\u2028' || ch == '\u2029' {
			return false
		}
	}
	return true
}
func (in CheckoutInfo) normalized() (CheckoutInfo, error) {
	if !validOrderEmail(in.Email) || !checkoutText(in.Name, 200, false) || !checkoutText(in.Phone, 64, false) {
		return in, ErrInvalidInfo
	}
	in.Email = strings.TrimSpace(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Country = strings.ToUpper(strings.TrimSpace(in.Country))
	if in.Name == "" || !knownCountry(in.Country) {
		return in, ErrInvalidInfo
	}
	if in.AddressDetails != nil {
		details, err := in.AddressDetails.Normalize(in.Country)
		if err != nil {
			return in, fmt.Errorf("%w: %w", ErrInvalidInfo, err)
		}
		in.AddressDetails = &details
		in.Address = details.Text()
		if in.ExpectedQuote == "" {
			return in, ErrQuoteChanged
		}
	} else {
		// Nil AddressDetails is the explicit legacy-service compatibility path.
		// Structured callers must supply details and a reviewed quote together.
		in.Address = strings.ReplaceAll(in.Address, "\r\n", "\n")
		if !checkoutText(in.Address, 2048, true) || strings.TrimSpace(in.Address) == "" {
			return in, ErrInvalidInfo
		}
		in.Address = strings.TrimSpace(in.Address)
	}
	if in.ExpectedQuote != "" {
		if value, err := hex.DecodeString(in.ExpectedQuote); err != nil || len(value) != 32 {
			return in, ErrQuoteChanged
		}
	}
	return in, nil
}
