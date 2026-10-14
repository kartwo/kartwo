// 登录限流测试 / Login Rate Limiter Tests
// 功能：验证大量不同用户名不会令限流状态无限增长
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 22:41:18
package admin

import (
	"strconv"
	"testing"
	"time"
)

func TestLoginLimiterBoundsDistinctKeys(t *testing.T) {
	l := newLoginLimiter(5, time.Minute)
	for i := 0; i < maxLimiterKeys+1; i++ {
		if !l.allow(strconv.Itoa(i)) {
			t.Fatalf("新 key %d 应允许首次尝试", i)
		}
	}
	if got := len(l.hits); got != maxLimiterKeys {
		t.Fatalf("限流状态有 %d 个 key，期望上限 %d", got, maxLimiterKeys)
	}
}
