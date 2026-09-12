// 店铺政策资料服务 / Store Policy Profile Service
// 功能：保存可编辑的经营资料，并据此幂等生成英文页脚内容页草稿或已发布页面
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 15:30:00
package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/settings"
)

const profileKey = "store.policy_profile"

var ErrInvalid = errors.New("policy: 店铺政策资料不合法")

var simpleEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type DeliveryEstimate struct {
	Region          string `json:"region"`
	MinBusinessDays int    `json:"min_business_days"`
	MaxBusinessDays int    `json:"max_business_days"`
}

type Profile struct {
	SupportEmail        string             `json:"support_email"`
	ShipFrom            string             `json:"ship_from"`
	ProcessingHours     int                `json:"processing_hours"`
	ReturnWindowDays    int                `json:"return_window_days"`
	ReturnShippingPayer string             `json:"return_shipping_payer"`
	BusinessName        string             `json:"business_name"`
	DeliveryEstimates   []DeliveryEstimate `json:"delivery_estimates"`
}

type GenerateResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type Service struct {
	settings *settings.Service
	catalog  *catalog.Service
}

func New(settingsSvc *settings.Service, catalogSvc *catalog.Service) *Service {
	return &Service{settings: settingsSvc, catalog: catalogSvc}
}

// KartwoDemoProfile 是显式 seed-running-demo 使用的演示资料，不在 serve/升级时自动写入。
func KartwoDemoProfile() Profile {
	return Profile{
		SupportEmail: "info@kartwo.com", ShipFrom: "China", ProcessingHours: 48,
		ReturnWindowDays: 7, ReturnShippingPayer: "buyer", BusinessName: "kartwo.com",
		DeliveryEstimates: []DeliveryEstimate{
			{Region: "Asia", MinBusinessDays: 5, MaxBusinessDays: 10},
			{Region: "Europe", MinBusinessDays: 7, MaxBusinessDays: 15},
			{Region: "North America", MinBusinessDays: 7, MaxBusinessDays: 15},
			{Region: "South America", MinBusinessDays: 10, MaxBusinessDays: 25},
			{Region: "Africa", MinBusinessDays: 10, MaxBusinessDays: 25},
			{Region: "Oceania", MinBusinessDays: 7, MaxBusinessDays: 15},
			{Region: "Antarctica", MinBusinessDays: 20, MaxBusinessDays: 35},
		},
	}
}

func EmptyProfile() Profile {
	p := KartwoDemoProfile()
	p.SupportEmail, p.ShipFrom, p.BusinessName = "", "", ""
	return p
}

func (s *Service) Get(ctx context.Context) (Profile, bool, error) {
	raw, err := s.settings.Get(ctx, profileKey)
	if errors.Is(err, settings.ErrNotFound) {
		return EmptyProfile(), false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	var p Profile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Profile{}, false, fmt.Errorf("policy: 解析店铺政策资料失败: %w", err)
	}
	return p, true, nil
}

func (s *Service) Save(ctx context.Context, p Profile) error {
	normalize(&p)
	if err := validate(p); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("policy: 编码店铺政策资料失败: %w", err)
	}
	return s.settings.SetPlain(ctx, profileKey, string(b))
}

func normalize(p *Profile) {
	p.SupportEmail = strings.TrimSpace(p.SupportEmail)
	p.ShipFrom = strings.TrimSpace(p.ShipFrom)
	p.BusinessName = strings.TrimSpace(p.BusinessName)
	p.ReturnShippingPayer = strings.TrimSpace(p.ReturnShippingPayer)
	for i := range p.DeliveryEstimates {
		p.DeliveryEstimates[i].Region = strings.TrimSpace(p.DeliveryEstimates[i].Region)
	}
}

func validate(p Profile) error {
	if len(p.SupportEmail) > 254 || !simpleEmail.MatchString(p.SupportEmail) {
		return fmt.Errorf("%w: 客服邮箱格式不正确", ErrInvalid)
	}
	if p.ShipFrom == "" || len([]rune(p.ShipFrom)) > 100 || p.BusinessName == "" || len([]rune(p.BusinessName)) > 200 {
		return fmt.Errorf("%w: 发货地和经营主体不能为空", ErrInvalid)
	}
	if p.ProcessingHours < 1 || p.ProcessingHours > 720 || p.ReturnWindowDays < 1 || p.ReturnWindowDays > 365 {
		return fmt.Errorf("%w: 处理时间或退货期限超出允许范围", ErrInvalid)
	}
	if p.ReturnShippingPayer != "buyer" && p.ReturnShippingPayer != "merchant" {
		return fmt.Errorf("%w: 退货运费承担方只能是买家或商家", ErrInvalid)
	}
	if len(p.DeliveryEstimates) != 7 {
		return fmt.Errorf("%w: 必须填写七个洲的预计配送时效", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, estimate := range p.DeliveryEstimates {
		if !validRegion(estimate.Region) || seen[estimate.Region] || estimate.MinBusinessDays < 1 || estimate.MaxBusinessDays < estimate.MinBusinessDays || estimate.MaxBusinessDays > 120 {
			return fmt.Errorf("%w: 配送时效地区或天数不正确", ErrInvalid)
		}
		seen[estimate.Region] = true
	}
	return nil
}

func validRegion(region string) bool {
	switch region {
	case "Asia", "Europe", "North America", "South America", "Africa", "Oceania", "Antarctica":
		return true
	default:
		return false
	}
}

type generatedPage struct{ title, slug, seo, body string }

func (s *Service) Generate(ctx context.Context, publish, overwrite bool) (GenerateResult, error) {
	p, configured, err := s.Get(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	if !configured {
		return GenerateResult{}, fmt.Errorf("%w: 请先保存店铺政策资料", ErrInvalid)
	}
	pages, err := s.catalog.ListContentPages(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	bySlug := make(map[string]catalog.ContentPage, len(pages))
	for _, page := range pages {
		bySlug[page.Slug] = page
	}
	result := GenerateResult{}
	for _, page := range generatedPages(p) {
		existing, found := bySlug[page.slug]
		if found && !overwrite {
			result.Skipped++
			continue
		}
		if found {
			if err := s.catalog.UpdateContentPage(ctx, existing.PublicID, page.title, page.body, page.seo, existing.Status); err != nil {
				return GenerateResult{}, err
			}
			result.Updated++
			continue
		}
		status := "draft"
		if publish {
			status = "active"
		}
		if _, err := s.catalog.CreateContentPage(ctx, page.title, page.slug, page.body, page.seo, status); err != nil {
			return GenerateResult{}, err
		}
		result.Created++
	}
	return result, nil
}

func generatedPages(p Profile) []generatedPage {
	payer := "the buyer"
	if p.ReturnShippingPayer == "merchant" {
		payer = "the merchant"
	}
	return []generatedPage{
		{"About Us", "about-us", "Learn about " + p.BusinessName + " and our cross-border running store.", fmt.Sprintf("%s is an independent cross-border store for runners. Orders are dispatched from %s.\n\nWe focus on practical running apparel, accessories, hydration and recovery essentials for everyday training.\n\nQuestions? Contact us at %s.", p.BusinessName, p.ShipFrom, p.SupportEmail)},
		{"Contact Us", "contact-us", "Contact " + p.BusinessName + " customer support.", fmt.Sprintf("For product, order, shipping or return questions, email us at %s.\n\nPlease include your order number when contacting us about an existing order. We will reply as soon as reasonably possible.", p.SupportEmail)},
		{"Shipping Policy", "shipping-policy", "Processing and estimated international delivery times for orders from " + p.BusinessName + ".", shippingBody(p)},
		{"Returns & Refunds", "returns-refunds", "Return window and return shipping information for " + p.BusinessName + ".", fmt.Sprintf("You may request a return within %d days after receiving your order.\n\nReturn shipping costs are paid by %s. This does not limit any rights you may have under applicable consumer law.\n\nBefore returning an item, contact %s with your order number and the reason for the return. We will provide the applicable return instructions.\n\nRefund eligibility is confirmed after the returned item is received and inspected. Approved refunds are issued to the original payment method; bank or payment-provider processing times may vary.", p.ReturnWindowDays, payer, p.SupportEmail)},
		{"Privacy Policy", "privacy-policy", "How " + p.BusinessName + " handles customer information.", fmt.Sprintf("%s uses the information you provide to operate the store, process orders, arrange delivery, provide support, prevent misuse and maintain required business records.\n\nThis information may include your name, contact details, delivery address, order details and payment-provider references. Full card details are handled by the selected payment provider and are not stored by this store.\n\nWe share information only when reasonably necessary with service providers involved in hosting, payment, delivery and customer support, or when required by law. Because this is a cross-border store, information may be processed in countries or regions different from your own.\n\nWe retain information only for as long as reasonably necessary for these purposes and applicable record-keeping obligations.\n\nFor privacy questions or requests, contact %s.", p.BusinessName, p.SupportEmail)},
		{"Terms of Service", "terms-of-service", "Terms for shopping with " + p.BusinessName + ".", fmt.Sprintf("These terms apply when you browse or place an order with %s. By placing an order, you confirm that the information you provide is accurate and that you are authorized to use the selected payment method.\n\nProduct availability, prices and delivery options may change before an order is accepted. An order is accepted when we send confirmation that it has been accepted or dispatched. We may cancel and refund an order if an item is unavailable, payment cannot be verified, delivery is not possible or fraud is reasonably suspected.\n\nThe Shipping Policy, Returns & Refunds Policy and Privacy Policy form part of these terms. Nothing in these terms limits rights that cannot lawfully be excluded.\n\nFor questions, contact %s.", p.BusinessName, p.SupportEmail)},
		{"FAQ", "faq", "Answers to common ordering, shipping and return questions.", fmt.Sprintf("## Where do orders ship from?\n\nOrders are dispatched from %s.\n\n## How long does processing take?\n\nOrders are normally processed within %d hours. Delivery estimates begin after dispatch.\n\n## How can I track my order?\n\nWhen tracking is available, it will be included in your shipping confirmation.\n\n## Can I return an order?\n\nYou may request a return within %d days after receipt. Please read our Returns & Refunds page before sending anything back.\n\n## How do I contact support?\n\nEmail %s and include your order number when applicable.", p.ShipFrom, p.ProcessingHours, p.ReturnWindowDays, p.SupportEmail)},
	}
}

func shippingBody(p Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Orders are dispatched from %s and are normally processed within %d hours.\n\nEstimated transit times after dispatch:\n", p.ShipFrom, p.ProcessingHours)
	for _, estimate := range p.DeliveryEstimates {
		fmt.Fprintf(&b, "\n- %s: %d–%d business days", estimate.Region, estimate.MinBusinessDays, estimate.MaxBusinessDays)
	}
	fmt.Fprintf(&b, "\n\nThese are estimates, not guarantees. Customs clearance, public holidays, weather, remote-area delivery and carrier disruptions may add time. Only countries or regions available at checkout can receive orders.\n\nWhen tracking is available, it will be sent after dispatch. For help, contact %s.", p.SupportEmail)
	return b.String()
}
