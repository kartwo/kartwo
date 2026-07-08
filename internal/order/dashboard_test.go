// 概览时区边界测试 / Dashboard Window Bounds Test
// 功能：锁死「今日/近7日」本地自然日→UTC 边界换算，跨 UTC 日界、跨时区下正确（防 false-green 回归）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-08 11:40:00
package order

import (
	"testing"
	"time"
)

// TestDashboardWindowBounds 用固定时区(东八区 UTC+8)+固定 now，确定性验证边界换算，
// 不依赖运行环境时区（CI 在 UTC 亦稳定）。重点：本地今日但 UTC 昨日的订单必须计入今日
// ——若实现退化为按 UTC 自然日切界，这些用例会失败。
func TestDashboardWindowBounds(t *testing.T) {
	cst := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, cst) // 本地 2026-07-08 上午 09:00

	todayBound, weekBound := dashboardWindowBounds(now)
	// 本地 07-08 零点 = UTC 07-07 16:00；本地(07-08 - 6天)零点 = UTC 07-01 16:00。
	if todayBound != "2026-07-07T16:00:00.000Z" {
		t.Fatalf("todayBound 应=2026-07-07T16:00:00.000Z（本地今日零点的 UTC），得 %q", todayBound)
	}
	if weekBound != "2026-07-01T16:00:00.000Z" {
		t.Fatalf("weekBound 应=2026-07-01T16:00:00.000Z，得 %q", weekBound)
	}

	// created_at（UTC 存储）与边界做与库内一致的词法比较；断言今日/近7日归属。
	cases := []struct {
		createdAt string
		desc      string
		inToday   bool
		inWeek    bool
	}{
		{"2026-07-07T18:00:00.000Z", "本地07-08 02:00（今日），UTC 是07-07（昨日）——必须计入今日", true, true},
		{"2026-07-08T02:05:58.019Z", "Derek 种子 o1（本地10:05 今日）", true, true},
		{"2026-07-07T15:59:59.000Z", "本地07-07 23:59（昨日），仍在近7日", false, true},
		{"2026-07-05T02:05:58.019Z", "Derek 种子 o4（3天前）", false, true},
		{"2026-07-01T15:59:59.000Z", "本地07-01 23:59，近7日窗口外", false, false},
	}
	for _, c := range cases {
		if got := c.createdAt >= todayBound; got != c.inToday {
			t.Fatalf("[%s] 计入今日应=%v，词法比较得=%v（todayBound=%s）", c.desc, c.inToday, got, todayBound)
		}
		if got := c.createdAt >= weekBound; got != c.inWeek {
			t.Fatalf("[%s] 计入近7日应=%v，得=%v（weekBound=%s）", c.desc, c.inWeek, got, weekBound)
		}
	}
}
