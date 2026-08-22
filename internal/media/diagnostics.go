// 媒体诊断统计 / Media Diagnostics
// 功能：汇总可见媒体占用，并读取媒体根目录所在文件系统的容量状态
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 09:10:00
package media

import (
	"context"
	"fmt"
)

// DiskUsage 是媒体根目录所在文件系统的容量快照。
type DiskUsage struct {
	Available  bool
	TotalBytes uint64
	FreeBytes  uint64
	UsedBytes  uint64
	Message    string
}

// Usage 是未删除媒体资产及其派生文件的数据库统计。
type Usage struct {
	AssetCount      int64
	OriginalBytes   int64
	DerivativeBytes int64
	TotalBytes      int64
	Disk            DiskUsage
}

// Diagnostics 汇总媒体记录和磁盘状态。磁盘探测失败不会阻断其它诊断项。
func (s *Service) Diagnostics(ctx context.Context) (Usage, error) {
	row, err := s.q.MediaUsage(ctx)
	if err != nil {
		return Usage{}, fmt.Errorf("media: 统计媒体占用失败: %w", err)
	}
	out := Usage{
		AssetCount:      row.AssetCount,
		OriginalBytes:   row.OriginalBytes,
		DerivativeBytes: row.DerivativeBytes,
		TotalBytes:      row.OriginalBytes + row.DerivativeBytes,
	}
	total, free, err := diskCapacity(s.backend.Root())
	if err != nil {
		out.Disk.Message = "暂时无法读取媒体磁盘容量"
		return out, nil
	}
	out.Disk = DiskUsage{Available: true, TotalBytes: total, FreeBytes: free, UsedBytes: total - free}
	return out, nil
}
