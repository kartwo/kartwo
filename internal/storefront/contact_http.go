// 店面客服表单 / Storefront Contact Form
// 功能：公开私密留言表单、可信来源 IP、同源限制和低成本防刷
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 14:10:00
package storefront

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kartwo/kartwo/internal/contact"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/policy"
)

type contactLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newContactLimiter() *contactLimiter { return &contactLimiter{hits: make(map[string][]time.Time)} }

func (l *contactLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 {
		for key, times := range l.hits {
			if len(times) == 0 || now.Sub(times[len(times)-1]) > time.Hour {
				delete(l.hits, key)
			}
		}
		if len(l.hits) > 10000 {
			return false
		}
	}
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 5 {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, now)
	return true
}

func (h *HTTP) contactPage(w http.ResponseWriter, r *http.Request) {
	h.renderContact(w, r, http.StatusOK, nil)
}

func (h *HTTP) renderContact(w http.ResponseWriter, r *http.Request, status int, input *contact.Input) {
	enabled := false
	cfg := contact.DefaultConfig()
	if h.contact != nil {
		var err error
		cfg, err = h.contact.Config(r.Context())
		if err != nil {
			http.Error(w, "Something went wrong", 500)
			return
		}
		enabled = cfg.Enabled
	}
	profile, _, err := policy.New(h.settings, nil).Get(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	shopName := h.shopName(r.Context())
	data := map[string]any{
		"ShopName": shopName, "SupportEmail": profile.SupportEmail, "Enabled": enabled, "Sent": r.URL.Query().Get("sent") == "1",
		"RetentionMonths": cfg.RetentionMonths, "IPRetentionDays": cfg.IPRetentionDays,
		"SEO": seo{Title: "Contact Us — " + shopName, Description: "Contact customer support at " + shopName, Canonical: h.base(r) + "/contact", OGType: "website"},
	}
	if input != nil {
		data["Name"], data["Email"], data["Topic"], data["OrderRef"], data["Body"] = input.Name, input.Email, input.Topic, input.OrderRef, input.Body
		data["Error"] = "Please check your details and try again."
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	h.render(w, r, h.contactTmpl, data)
}

func (h *HTTP) sameOrigin(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	expectedScheme := "http"
	if h.secureFor(r) {
		expectedScheme = "https"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) || u.Scheme != expectedScheme {
			return false
		}
		return true
	}
	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

func (h *HTTP) peerTrusted(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, network := range h.trusted {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *HTTP) contactIPCountry(r *http.Request) (string, string) {
	ip := remoteIP(r)
	trusted := h.peerTrusted(ip)
	if trusted {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := net.ParseIP(strings.TrimSpace(parts[i]))
			if candidate == nil {
				break
			}
			ip = candidate
			if !h.peerTrusted(candidate) {
				break
			}
		}
	}
	ipText := ""
	if ip != nil {
		ipText = ip.String()
	}
	// 只信任明确配置过的代理提供的国家码；直接访问时绝不采信访客自带的头。
	country := ""
	if trusted {
		candidate := strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
		for _, item := range order.CountryOptions() {
			if item.Code == candidate {
				country = candidate
				break
			}
		}
	}
	return ipText, country
}

func (h *HTTP) contactSubmit(w http.ResponseWriter, r *http.Request) {
	if h.contact == nil {
		http.Error(w, "Messages are unavailable", http.StatusForbidden)
		return
	}
	cfg, err := h.contact.Config(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	if !cfg.Enabled {
		http.Error(w, "Messages are unavailable", http.StatusForbidden)
		return
	}
	if !h.sameOrigin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Error(w, "Unsupported form", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	if r.PostForm.Get("website") != "" {
		http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
		return
	}
	ip, country := h.contactIPCountry(r)
	if !h.contactLimit.allow(ip) {
		http.Error(w, "Please try again later", http.StatusTooManyRequests)
		return
	}
	in := contact.Input{Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"), Topic: r.PostForm.Get("topic"), Body: r.PostForm.Get("body"), OrderRef: r.PostForm.Get("order_ref"), IP: ip, Country: country}
	if _, err := h.contact.Submit(r.Context(), in); err != nil {
		if errors.Is(err, contact.ErrInvalid) {
			h.renderContact(w, r, http.StatusBadRequest, &in)
			return
		}
		if errors.Is(err, contact.ErrDisabled) {
			http.Error(w, "Messages are unavailable", http.StatusForbidden)
			return
		}
		h.renderContact(w, r, http.StatusInternalServerError, &in)
		return
	}
	http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
}
