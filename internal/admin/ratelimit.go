// 登录限流 / Login Rate Limiter
// 功能：有界内存滑动窗口限流，限制后台登录及公开入口的尝试频率
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"sync"
	"time"
)

const maxLimiterKeys = 4096

// loginLimiter 为单实例自部署使用的有界内存滑动窗口限流器。
type loginLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
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
	if _, exists := l.hits[key]; !exists && len(l.hits) >= maxLimiterKeys {
		oldestKey := ""
		var oldestTime time.Time
		for candidate, times := range l.hits {
			if len(times) == 0 || !times[len(times)-1].After(cutoff) {
				delete(l.hits, candidate)
				continue
			}
			if oldestKey == "" || times[0].Before(oldestTime) {
				oldestKey, oldestTime = candidate, times[0]
			}
		}
		if len(l.hits) >= maxLimiterKeys {
			delete(l.hits, oldestKey)
		}
	}
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
