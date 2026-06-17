// 登录限流 / Login Rate Limiter
// 功能：内存固定窗口限流，按 key(IP|用户名) 限制登录尝试，防爆破
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"sync"
	"time"
)

// loginLimiter 为内存固定窗口限流器（单实例自部署足够）。
type loginLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	max     int
	window  time.Duration
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{hits: make(map[string][]time.Time), max: max, window: window}
}

// allow 记录一次尝试；窗口内超过 max 次则拒绝（返回 false）。
func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// reset 清除某 key 的计数（登录成功后调用）。
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
