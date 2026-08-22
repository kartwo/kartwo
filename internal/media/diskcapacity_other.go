// 磁盘容量（非 Unix）/ Disk Capacity (Non-Unix)
// 功能：非 Unix 平台明确返回不可用，避免在诊断页伪造容量数值
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 09:10:00
//go:build !unix

package media

import "errors"

func diskCapacity(string) (uint64, uint64, error) {
	return 0, 0, errors.New("磁盘容量探测暂不支持当前平台")
}
