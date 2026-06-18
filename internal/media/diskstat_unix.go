// 磁盘可用空间（Unix）/ Disk Free Space (Unix)
// 功能：统计某路径所在文件系统的可用字节（darwin/linux）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
//go:build unix

package media

import "syscall"

// freeBytes 返回 path 所在文件系统的可用字节数。
func freeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec,unconvert // 跨 darwin/linux 字段类型差异，统一转 uint64
}
