// 磁盘可用空间（非 Unix）/ Disk Free Space (non-Unix)
// 功能：非 Unix 平台的兜底实现，不阻塞上传（返回足够大）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
//go:build !unix

package media

import "math"

// freeBytes 在非 Unix 平台返回极大值（不触发磁盘护栏）。
func freeBytes(_ string) (uint64, error) {
	return math.MaxUint64, nil
}
