// 店面交易入口保护 / Storefront Commerce Guards
// 功能：有界内存限流和请求体上限，在数据库写入及外部支付调用之前拦截过量请求
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 13:29:27
package storefront

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kartwo/kartwo/internal/httpx"
)

const commerceMaxBody = 64 << 10
const commerceMaxKeys = 4096

type commerceWindow struct {
	count int
	until time.Time
}
type commerceLimiter struct {
	mu        sync.Mutex
	entries   map[string]commerceWindow
	max       int
	lastSweep time.Time
}

func newCommerceLimiter(max int) *commerceLimiter {
	return &commerceLimiter{entries: make(map[string]commerceWindow), max: max}
}

// allowAt 的时间由调用方提供，便于不依赖 sleep 验证边界。满容量时拒绝新键，不驱逐正在限流的来源。
func (l *commerceLimiter) allowAt(key string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= time.Minute {
		for k, entry := range l.entries {
			if !now.Before(entry.until) {
				delete(l.entries, k)
			}
		}
		l.lastSweep = now
	}
	entry, exists := l.entries[key]
	if exists && !now.Before(entry.until) {
		delete(l.entries, key)
		entry = commerceWindow{}
		exists = false
	}
	if !exists {
		if len(l.entries) >= commerceMaxKeys {
			return false, 60
		}
		entry.until = now.Add(time.Minute)
	}
	if entry.count >= l.max {
		retry := int(entry.until.Sub(now).Seconds()) + 1
		return false, retry
	}
	entry.count++
	l.entries[key] = entry
	return true, 0
}

func (h *HTTP) protectCommerce(limiter *commerceLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.PrivateHeaders(w)
		// PayPal's authenticated provider return remains a cross-site GET.
		// Every browser-initiated write must carry a same-origin signal.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.sameOrigin(r) {
			writeErr(w, http.StatusForbidden, "Please submit this request from the store page.")
			return
		}
		ip, _ := h.contactIPCountry(r) // 仅采信已配置代理，不能用伪造 X-Forwarded-For 绕过。
		if allowed, retry := limiter.allowAt(ip, time.Now()); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeErr(w, http.StatusTooManyRequests, "Too many requests. Please wait a moment and try again.")
			return
		}
		if r.ContentLength > commerceMaxBody {
			writeErr(w, http.StatusRequestEntityTooLarge, "Request is too large.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, commerceMaxBody)
		next(w, r)
	}
}

// parseCommerceForm 覆盖 Content-Length 缺失的分块请求，由 MaxBytesReader 实际限制读取量。
func parseCommerceForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeErr(w, status, "Invalid or oversized form.")
		return false
	}
	return true
}
