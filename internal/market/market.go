// 市场框架 / Market Framework
// 功能：可扩展的"市场"定义与注册表；v1 只点亮美国，其余标"即将上线"。加新市场=插一条配置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-19 21:22:05
package market

// Status 市场状态。
type Status string

const (
	// Available 已点亮可用。
	Available Status = "available"
	// ComingSoon 即将上线（占位，未点亮）。
	ComingSoon Status = "coming_soon"
)

// Market 是一个市场的全部"按市场配置"项。未来加市场只需往 registry 插一条。
type Market struct {
	Code      string   // 唯一代码，如 US
	Name      string   // 展示名（双语）
	Currency  string   // 货币代码，如 USD
	Locale    string   // 语言，如 en
	RTL       bool     // 是否从右到左
	Providers []string // 支付通道，如 ["stripe","paypal"]
	Status    Status
	Enables   string // 大白话："选了会启用什么"
	Note      string // coming_soon 的正向提示
}

// registry 为有序市场列表（展示顺序）。v1 仅 US=available。
var registry = []Market{
	{
		Code: "US", Name: "美国 / United States", Currency: "USD", Locale: "en", RTL: false,
		Providers: []string{"stripe", "paypal"}, Status: Available,
		Enables: "Stripe、PayPal 收款 · 美元（USD）· 英语店面",
	},
	{
		Code: "EU", Name: "欧洲 / Europe", Currency: "EUR", Locale: "en", RTL: false,
		Providers: []string{"stripe", "paypal"}, Status: ComingSoon,
		Enables: "iDEAL、Klarna 等本地支付 · 欧元（EUR）· 多语言",
		Note:    "可先用美国开店，欧洲上线后一键切换。",
	},
	{
		Code: "MENA", Name: "中东 / Middle East", Currency: "AED", Locale: "ar", RTL: true,
		Providers: []string{}, Status: ComingSoon,
		Enables: "Mada、Tabby 等本地支付 · 本地货币 · 阿拉伯语（从右到左排版）",
		Note:    "可先用美国开店，中东上线后再切换。",
	},
	{
		Code: "SEA", Name: "东南亚 / Southeast Asia", Currency: "SGD", Locale: "en", RTL: false,
		Providers: []string{}, Status: ComingSoon,
		Enables: "本地钱包与分期 · 本地货币 · 多语言",
		Note:    "可先用美国开店，东南亚上线后再切换。",
	},
	{
		Code: "LATAM", Name: "拉丁美洲 / Latin America", Currency: "BRL", Locale: "pt", RTL: false,
		Providers: []string{}, Status: ComingSoon,
		Enables: "本地支付与分期 · 本地货币 · 多语言",
		Note:    "可先用美国开店，拉美上线后再切换。",
	},
}

// List 返回全部市场（含即将上线）。
func List() []Market {
	out := make([]Market, len(registry))
	copy(out, registry)
	return out
}

// Lookup 按代码取市场。
func Lookup(code string) (Market, bool) {
	for _, m := range registry {
		if m.Code == code {
			return m, true
		}
	}
	return Market{}, false
}

// Default 返回默认市场（美国）。
func Default() Market {
	m, _ := Lookup("US")
	return m
}

// IsAvailable 报告某市场是否已点亮可选。
func IsAvailable(code string) bool {
	m, ok := Lookup(code)
	return ok && m.Status == Available
}
