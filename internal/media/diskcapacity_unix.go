// 磁盘容量（Unix）/ Disk Capacity (Unix)
// 功能：读取媒体根目录所在文件系统的总容量和可用容量
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 09:10:00
//go:build unix

package media

import "syscall"

func diskCapacity(path string) (uint64, uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	blockSize := uint64(st.Bsize)
	return uint64(st.Blocks) * blockSize, uint64(st.Bavail) * blockSize, nil
}
