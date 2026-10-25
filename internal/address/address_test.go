// 地址规则测试 / Address Rule Tests
// 功能：验证多国字段差异、全部离线规则、邮编语法、Unicode 和错误脱敏
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 17:18:27
package address

import (
	"errors"
	"strings"
	"testing"
)

func sample() Details {
	return Details{Line1: "1 Fixture Street", City: "Fixture City", Region: "CA", PostalCode: "94043"}
}
func required(profile Profile, key string) bool {
	for _, field := range profile.Fields {
		if field.Key == key {
			return field.Required
		}
	}
	return false
}

func TestCountryProfiles(t *testing.T) {
	us, ok := ForCountry(" us ")
	if !ok || us.Country != "US" || !required(us, "city") || !required(us, "region") || !required(us, "postal_code") {
		t.Fatalf("US fields: %+v", us)
	}
	gb, _ := ForCountry("GB")
	de, _ := ForCountry("DE")
	hk, _ := ForCountry("HK")
	sg, _ := ForCountry("SG")
	if required(gb, "region") || required(de, "region") || required(hk, "postal_code") || required(sg, "city") {
		t.Fatal("US requirements leaked to other destinations")
	}
	if !required(hk, "region") || !required(sg, "postal_code") {
		t.Fatal("country requirements missing")
	}
	for _, code := range []string{"", "ZZ", "XX", "US/CA", "<script>"} {
		if _, ok := ForCountry(code); ok {
			t.Fatalf("accepted country %q", code)
		}
	}
}

func TestCountryPostalSyntax(t *testing.T) {
	for _, test := range []struct {
		country, postal string
		valid           bool
	}{
		{"US", "94043", true}, {"US", "94043-1234", true}, {"US", "abcd", false},
		{"CA", "k1a 0b1", true}, {"CA", "12345", false}, {"GB", "sw1a 1aa", true},
		{"DE", "10115", true}, {"DE", "1011", false}, {"FR", "75001", true},
		{"AU", "2000", true}, {"CN", "100000", true}, {"SG", "018956", true},
	} {
		t.Run(test.country+test.postal, func(t *testing.T) {
			in := sample()
			in.PostalCode = test.postal
			_, err := in.Normalize(test.country)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
		})
	}
	in := Details{Line1: "Fixture Road", Region: "香港島"}
	if _, err := in.Normalize("HK"); err != nil {
		t.Fatal(err)
	}
}

func TestAllOfflineRuleExamples(t *testing.T) {
	if len(rules) < 240 {
		t.Fatalf("incomplete global rules: %d", len(rules))
	}
	for code := range rules {
		if code == "ZZ" {
			continue
		}
		profile, ok := ForCountry(code)
		if !ok {
			t.Fatalf("profile unavailable: %s", code)
		}
		in := sample()
		in.District = "Fixture district"
		if profile.PostalExample != "" {
			in.PostalCode = profile.PostalExample
		} else {
			in.PostalCode = ""
		}
		// Some countries have a required postcode but no example. Check field
		// requirements separately; never invent a valid postal code for them.
		if required(profile, "postal_code") && profile.PostalExample == "" {
			continue
		}
		if _, err := in.Normalize(code); err != nil {
			t.Errorf("published example for %s: %v", code, err)
		}
	}
}

func TestAddressBoundsUnicodeAndPrivacy(t *testing.T) {
	in := sample()
	in.Line1 = "  東京都 Fixture 1番地  "
	in.Line2 = "Apt 2"
	out, err := in.Normalize("US")
	if err != nil || out.Line1 != "東京都 Fixture 1番地" || !strings.Contains(out.Text(), "Apt 2") {
		t.Fatalf("normalization failed: %v", err)
	}
	for _, bad := range []string{"PRIVATE\nINJECTION", "PRIVATE\x00ADDRESS", strings.Repeat("地", 90), string([]byte{0xff})} {
		in = sample()
		in.Line1 = bad
		_, err = in.Normalize("US")
		var field *FieldError
		if !errors.As(err, &field) || field.Field != "line1" || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatalf("invalid address/error privacy: %v", err)
		}
	}
	in = sample()
	in.PostalCode = ""
	if _, err = in.Normalize("US"); err == nil {
		t.Fatal("missing required postcode accepted")
	}
	in = sample()
	in.Line2 = "PRIVATE\tINJECTION"
	if _, err = in.Normalize("US"); err == nil {
		t.Fatal("optional field bypassed validation")
	}
}
