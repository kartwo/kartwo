// 国家地区目录 / Country and Region Catalog
// 功能：提供自部署结账与配送设置共用的 ISO 3166-1 alpha-2 选择项
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-08 14:10:00
package order

import "strings"

// Country 是配送设置和结账页使用的国家/地区选项。
type Country struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Continent string `json:"continent"`
}

// CountryOptions 返回稳定的 ISO 3166-1 alpha-2 国家/地区目录副本。
// 名称采用 IANA tz database 的通用英文名称（public domain），以免引入运行时外部依赖。
func CountryOptions() []Country {
	out := make([]Country, 0, len(countryOptions))
	for _, country := range countryOptions {
		out = append(out, Country{Code: country.Code, Name: country.Name, Continent: continentFor(country.Code)})
	}
	return out
}

func knownCountry(code string) bool {
	for _, country := range countryOptions {
		if country.Code == code {
			return true
		}
	}
	return false
}

func normalizeCountries(raw string) (string, bool) {
	seen := map[string]bool{}
	values := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		code := strings.ToUpper(strings.TrimSpace(value))
		if code == "" {
			continue
		}
		if !knownCountry(code) {
			return "", false
		}
		if !seen[code] {
			seen[code] = true
			values = append(values, code)
		}
	}
	return strings.Join(values, ","), true
}

// NormalizeShippingCountries 验证并规范化后台提交的国家/地区代码列表。
func NormalizeShippingCountries(raw string) (string, bool) {
	return normalizeCountries(raw)
}

type countryRecord struct{ Code, Name string }

func continentFor(code string) string {
	for continent, codes := range continentCodes {
		if strings.Contains(" "+codes+" ", " "+code+" ") {
			return continent
		}
	}
	return "Other"
}

var continentCodes = map[string]string{
	"Africa":        "DZ AO BJ BW BF BI CV CM CF TD KM CG CD CI DJ EG GQ ER SZ ET GA GM GH GN GW KE LS LR LY MG MW ML MR MU YT MA MZ NA NE NG RE RW SH ST SN SC SL SO ZA SS SD TZ TG TN UG EH ZM ZW",
	"Asia":          "AF AM AZ BH BD BT BN KH CN CY GE HK IN ID IO IR IQ IL JP JO KZ KP KR KW KG LA LB MO MY MV MN MM NP OM PK PS PH QA SA SG LK SY TW TJ TH TL TR TM AE UZ VN YE",
	"Europe":        "AX AL AD AT BY BE BA BG HR CZ DK EE FO FI FR DE GI GR GG VA HU IS IE IM IT JE LV LI LT LU MT MD MC ME NL MK NO PL PT RO RU SM RS SK SI ES SJ SE CH UA GB",
	"North America": "AI AG AW BS BB BZ BM BQ CA KY CR CU CW DM DO SV GL GD GP GT HT HN JM MQ MX MS NI PA PR BL KN LC MF PM VC SX TT TC US VG VI",
	"South America": "AR BO BV BR CL CO EC FK GF GY PY PE GS SR UY VE",
	"Oceania":       "AS AU CC CX CK FJ PF GU HM KI MH FM NR NC NZ NU NF MP PW PG PN WS SB TK TO TV UM VU WF",
	"Antarctica":    "AQ TF",
}

var countryOptions = []countryRecord{
	{"AD", "Andorra"}, {"AE", "United Arab Emirates"}, {"AF", "Afghanistan"}, {"AG", "Antigua & Barbuda"}, {"AI", "Anguilla"}, {"AL", "Albania"}, {"AM", "Armenia"}, {"AO", "Angola"}, {"AQ", "Antarctica"}, {"AR", "Argentina"}, {"AS", "American Samoa"}, {"AT", "Austria"}, {"AU", "Australia"}, {"AW", "Aruba"}, {"AX", "Åland Islands"}, {"AZ", "Azerbaijan"},
	{"BA", "Bosnia & Herzegovina"}, {"BB", "Barbados"}, {"BD", "Bangladesh"}, {"BE", "Belgium"}, {"BF", "Burkina Faso"}, {"BG", "Bulgaria"}, {"BH", "Bahrain"}, {"BI", "Burundi"}, {"BJ", "Benin"}, {"BL", "Saint Barthélemy"}, {"BM", "Bermuda"}, {"BN", "Brunei"}, {"BO", "Bolivia"}, {"BQ", "Caribbean Netherlands"}, {"BR", "Brazil"}, {"BS", "Bahamas"}, {"BT", "Bhutan"}, {"BV", "Bouvet Island"}, {"BW", "Botswana"}, {"BY", "Belarus"}, {"BZ", "Belize"},
	{"CA", "Canada"}, {"CC", "Cocos (Keeling) Islands"}, {"CD", "Congo (Democratic Republic)"}, {"CF", "Central African Republic"}, {"CG", "Congo (Republic)"}, {"CH", "Switzerland"}, {"CI", "Côte d’Ivoire"}, {"CK", "Cook Islands"}, {"CL", "Chile"}, {"CM", "Cameroon"}, {"CN", "China"}, {"CO", "Colombia"}, {"CR", "Costa Rica"}, {"CU", "Cuba"}, {"CV", "Cape Verde"}, {"CW", "Curaçao"}, {"CX", "Christmas Island"}, {"CY", "Cyprus"}, {"CZ", "Czech Republic"},
	{"DE", "Germany"}, {"DJ", "Djibouti"}, {"DK", "Denmark"}, {"DM", "Dominica"}, {"DO", "Dominican Republic"}, {"DZ", "Algeria"}, {"EC", "Ecuador"}, {"EE", "Estonia"}, {"EG", "Egypt"}, {"EH", "Western Sahara"}, {"ER", "Eritrea"}, {"ES", "Spain"}, {"ET", "Ethiopia"},
	{"FI", "Finland"}, {"FJ", "Fiji"}, {"FK", "Falkland Islands"}, {"FM", "Micronesia"}, {"FO", "Faroe Islands"}, {"FR", "France"}, {"GA", "Gabon"}, {"GB", "United Kingdom"}, {"GD", "Grenada"}, {"GE", "Georgia"}, {"GF", "French Guiana"}, {"GG", "Guernsey"}, {"GH", "Ghana"}, {"GI", "Gibraltar"}, {"GL", "Greenland"}, {"GM", "Gambia"}, {"GN", "Guinea"}, {"GP", "Guadeloupe"}, {"GQ", "Equatorial Guinea"}, {"GR", "Greece"}, {"GS", "South Georgia & South Sandwich Islands"}, {"GT", "Guatemala"}, {"GU", "Guam"}, {"GW", "Guinea-Bissau"}, {"GY", "Guyana"},
	{"HK", "Hong Kong"}, {"HM", "Heard Island & McDonald Islands"}, {"HN", "Honduras"}, {"HR", "Croatia"}, {"HT", "Haiti"}, {"HU", "Hungary"}, {"ID", "Indonesia"}, {"IE", "Ireland"}, {"IL", "Israel"}, {"IM", "Isle of Man"}, {"IN", "India"}, {"IO", "British Indian Ocean Territory"}, {"IQ", "Iraq"}, {"IR", "Iran"}, {"IS", "Iceland"}, {"IT", "Italy"},
	{"JE", "Jersey"}, {"JM", "Jamaica"}, {"JO", "Jordan"}, {"JP", "Japan"}, {"KE", "Kenya"}, {"KG", "Kyrgyzstan"}, {"KH", "Cambodia"}, {"KI", "Kiribati"}, {"KM", "Comoros"}, {"KN", "Saint Kitts & Nevis"}, {"KP", "North Korea"}, {"KR", "South Korea"}, {"KW", "Kuwait"}, {"KY", "Cayman Islands"}, {"KZ", "Kazakhstan"},
	{"LA", "Laos"}, {"LB", "Lebanon"}, {"LC", "Saint Lucia"}, {"LI", "Liechtenstein"}, {"LK", "Sri Lanka"}, {"LR", "Liberia"}, {"LS", "Lesotho"}, {"LT", "Lithuania"}, {"LU", "Luxembourg"}, {"LV", "Latvia"}, {"LY", "Libya"},
	{"MA", "Morocco"}, {"MC", "Monaco"}, {"MD", "Moldova"}, {"ME", "Montenegro"}, {"MF", "Saint Martin (French)"}, {"MG", "Madagascar"}, {"MH", "Marshall Islands"}, {"MK", "North Macedonia"}, {"ML", "Mali"}, {"MM", "Myanmar"}, {"MN", "Mongolia"}, {"MO", "Macao"}, {"MP", "Northern Mariana Islands"}, {"MQ", "Martinique"}, {"MR", "Mauritania"}, {"MS", "Montserrat"}, {"MT", "Malta"}, {"MU", "Mauritius"}, {"MV", "Maldives"}, {"MW", "Malawi"}, {"MX", "Mexico"}, {"MY", "Malaysia"}, {"MZ", "Mozambique"},
	{"NA", "Namibia"}, {"NC", "New Caledonia"}, {"NE", "Niger"}, {"NF", "Norfolk Island"}, {"NG", "Nigeria"}, {"NI", "Nicaragua"}, {"NL", "Netherlands"}, {"NO", "Norway"}, {"NP", "Nepal"}, {"NR", "Nauru"}, {"NU", "Niue"}, {"NZ", "New Zealand"},
	{"OM", "Oman"}, {"PA", "Panama"}, {"PE", "Peru"}, {"PF", "French Polynesia"}, {"PG", "Papua New Guinea"}, {"PH", "Philippines"}, {"PK", "Pakistan"}, {"PL", "Poland"}, {"PM", "Saint Pierre & Miquelon"}, {"PN", "Pitcairn"}, {"PR", "Puerto Rico"}, {"PS", "Palestine"}, {"PT", "Portugal"}, {"PW", "Palau"}, {"PY", "Paraguay"},
	{"QA", "Qatar"}, {"RE", "Réunion"}, {"RO", "Romania"}, {"RS", "Serbia"}, {"RU", "Russia"}, {"RW", "Rwanda"}, {"SA", "Saudi Arabia"}, {"SB", "Solomon Islands"}, {"SC", "Seychelles"}, {"SD", "Sudan"}, {"SE", "Sweden"}, {"SG", "Singapore"}, {"SH", "Saint Helena"}, {"SI", "Slovenia"}, {"SJ", "Svalbard & Jan Mayen"}, {"SK", "Slovakia"}, {"SL", "Sierra Leone"}, {"SM", "San Marino"}, {"SN", "Senegal"}, {"SO", "Somalia"}, {"SR", "Suriname"}, {"SS", "South Sudan"}, {"ST", "São Tomé & Príncipe"}, {"SV", "El Salvador"}, {"SX", "Saint Martin (Dutch)"}, {"SY", "Syria"}, {"SZ", "Eswatini"},
	{"TC", "Turks & Caicos Islands"}, {"TD", "Chad"}, {"TF", "French Southern Territories"}, {"TG", "Togo"}, {"TH", "Thailand"}, {"TJ", "Tajikistan"}, {"TK", "Tokelau"}, {"TL", "Timor-Leste"}, {"TM", "Turkmenistan"}, {"TN", "Tunisia"}, {"TO", "Tonga"}, {"TR", "Türkiye"}, {"TT", "Trinidad & Tobago"}, {"TV", "Tuvalu"}, {"TW", "Taiwan"}, {"TZ", "Tanzania"},
	{"UA", "Ukraine"}, {"UG", "Uganda"}, {"UM", "United States Minor Outlying Islands"}, {"US", "United States"}, {"UY", "Uruguay"}, {"UZ", "Uzbekistan"}, {"VA", "Vatican City"}, {"VC", "Saint Vincent"}, {"VE", "Venezuela"}, {"VG", "British Virgin Islands"}, {"VI", "U.S. Virgin Islands"}, {"VN", "Vietnam"}, {"VU", "Vanuatu"}, {"WF", "Wallis & Futuna"}, {"WS", "Samoa"}, {"YE", "Yemen"}, {"YT", "Mayotte"}, {"ZA", "South Africa"}, {"ZM", "Zambia"}, {"ZW", "Zimbabwe"},
}
