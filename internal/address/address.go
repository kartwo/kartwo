// 多国收货地址 / International Delivery Address
// 功能：离线国家字段要求、邮编语法与结构化地址校验；不做地址真实性或投递保证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 17:16:44
package address

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed rules.json
var ruleData []byte

type metadata struct {
	Format       string `json:"fmt"`
	LatinFormat  string `json:"lfmt"`
	Required     string `json:"require"`
	Postal       string `json:"zip"`
	Examples     string `json:"zipex"`
	RegionType   string `json:"state_name_type"`
	CityType     string `json:"locality_name_type"`
	DistrictType string `json:"sublocality_name_type"`
	PostalType   string `json:"zip_name_type"`
}

type countryRule struct {
	metadata
	postal *regexp.Regexp
}

var rules = loadRules()

func loadRules() map[string]countryRule {
	var file struct {
		Countries map[string]metadata `json:"countries"`
	}
	if err := json.Unmarshal(ruleData, &file); err != nil {
		panic("invalid embedded address rules")
	}
	base, ok := file.Countries["ZZ"]
	if !ok || base.Required == "" || base.Format == "" {
		panic("missing default address rules")
	}
	out := make(map[string]countryRule, len(file.Countries))
	for code, entry := range file.Countries {
		if entry.Required == "" {
			entry.Required = base.Required
		}
		if entry.Format == "" {
			entry.Format = base.Format
		}
		if entry.RegionType == "" {
			entry.RegionType = base.RegionType
		}
		if entry.CityType == "" {
			entry.CityType = base.CityType
		}
		if entry.DistrictType == "" {
			entry.DistrictType = base.DistrictType
		}
		if entry.PostalType == "" {
			entry.PostalType = base.PostalType
		}
		r := countryRule{metadata: entry}
		if entry.Postal != "" {
			r.postal = regexp.MustCompile("(?i)^(?:" + entry.Postal + ")$")
		}
		out[code] = r
	}
	return out
}

// Details is a per-order snapshot, not a mutable customer address book.
type Details struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	District   string `json:"district"`
	Region     string `json:"region"`
	PostalCode string `json:"postal_code"`
}

type Field struct {
	Key, Label     string
	Required, Used bool
	MaxBytes       int
}
type Profile struct {
	Country       string
	Fields        []Field
	PostalExample string
}

var labels = map[string]string{
	"city": "City", "post_town": "Post town", "suburb": "Suburb", "district": "District",
	"province": "Province", "state": "State", "emirate": "Emirate", "area": "Area", "island": "Island",
	"county": "County", "department": "Department", "oblast": "Region", "prefecture": "Prefecture", "parish": "Parish",
	"neighborhood": "Neighborhood", "village_township": "Village / township", "zip": "ZIP code", "postal": "Postal code",
	"eircode": "Eircode", "pin": "PIN code",
}

func label(kind, fallback string) string {
	if value := labels[kind]; value != "" {
		return value
	}
	return fallback
}

// ForCountry returns all fields without enabling this country for shipping.
func ForCountry(country string) (Profile, bool) {
	code := strings.ToUpper(strings.TrimSpace(country))
	r, ok := rules[code]
	if !ok || code == "ZZ" {
		return Profile{}, false
	}
	format := r.Format + r.LatinFormat
	fields := []Field{
		{Key: "line1", Label: "Street address", Required: true, Used: true, MaxBytes: 256},
		{Key: "line2", Label: "Apartment, suite, unit (optional)", Used: true, MaxBytes: 256},
		{Key: "district", Label: label(r.DistrictType, "District / neighborhood"), Required: strings.Contains(r.Required, "D"), Used: strings.Contains(format, "%D"), MaxBytes: 128},
		{Key: "city", Label: label(r.CityType, "City / locality"), Required: strings.Contains(r.Required, "C"), Used: strings.Contains(format, "%C"), MaxBytes: 128},
		{Key: "region", Label: label(r.RegionType, "State / province / region"), Required: strings.Contains(r.Required, "S"), Used: strings.Contains(format, "%S"), MaxBytes: 128},
		{Key: "postal_code", Label: label(r.PostalType, "Postal code"), Required: strings.Contains(r.Required, "Z"), Used: strings.Contains(format, "%Z"), MaxBytes: 32},
	}
	return Profile{Country: code, Fields: fields, PostalExample: strings.Split(r.Examples, ",")[0]}, true
}

// FieldError never contains submitted address values, even in server logs.
type FieldError struct{ Field, Message string }

func (e *FieldError) Error() string { return e.Message }

var ErrCountry = errors.New("select a supported country or region")

func validText(value string, maxBytes int) bool {
	if !utf8.ValidString(value) || len(value) > maxBytes {
		return false
	}
	for _, ch := range value {
		if unicode.IsControl(ch) || ch == '\u2028' || ch == '\u2029' {
			return false
		}
	}
	return true
}

// Normalize checks all supplied fields, including optional fields, without
// silently discarding characters or looking up the buyer at an external API.
func (in Details) Normalize(country string) (Details, error) {
	profile, ok := ForCountry(country)
	if !ok {
		return Details{}, ErrCountry
	}
	raw := map[string]string{"line1": in.Line1, "line2": in.Line2, "city": in.City, "district": in.District, "region": in.Region, "postal_code": in.PostalCode}
	for _, field := range profile.Fields {
		if !validText(raw[field.Key], field.MaxBytes) {
			return Details{}, &FieldError{field.Key, fmt.Sprintf("%s is too long or contains unsupported control characters.", field.Label)}
		}
	}
	in.Line1 = strings.TrimSpace(in.Line1)
	in.Line2 = strings.TrimSpace(in.Line2)
	in.City = strings.TrimSpace(in.City)
	in.District = strings.TrimSpace(in.District)
	in.Region = strings.TrimSpace(in.Region)
	in.PostalCode = strings.ToUpper(strings.TrimSpace(in.PostalCode))
	values := map[string]string{"line1": in.Line1, "line2": in.Line2, "city": in.City, "district": in.District, "region": in.Region, "postal_code": in.PostalCode}
	for _, field := range profile.Fields {
		value := values[field.Key]
		if !validText(value, field.MaxBytes) {
			return Details{}, &FieldError{field.Key, fmt.Sprintf("%s is too long or contains unsupported control characters.", field.Label)}
		}
		if field.Required && value == "" {
			return Details{}, &FieldError{field.Key, fmt.Sprintf("%s is required for this country or region.", field.Label)}
		}
	}
	if r := rules[profile.Country]; in.PostalCode != "" && r.postal != nil && !r.postal.MatchString(in.PostalCode) {
		return Details{}, &FieldError{"postal_code", "Please check the postal code format for the selected country or region."}
	}
	return in, nil
}

// Text retains a readable compatibility snapshot for older exports and views.
func (in Details) Text() string {
	var lines []string
	for _, value := range []string{in.Line1, in.Line2, in.District, in.City, in.Region, in.PostalCode} {
		if value != "" {
			lines = append(lines, value)
		}
	}
	return strings.Join(lines, "\n")
}
