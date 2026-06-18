// 存储策略与磁盘护栏 / Storage Policy & Disk Guard
// 功能：StoragePolicy 接口（双模式分叉点）+ 默认实现（不限额，仅单文件与磁盘护栏）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
package media

import "errors"

var (
	// ErrFileTooLarge 单文件超过上限。
	ErrFileTooLarge = errors.New("media: 文件超过大小上限")
	// ErrDiskFull 磁盘将满，停新上传（优雅降级，不破坏已有数据）。
	ErrDiskFull = errors.New("media: 磁盘空间不足，暂停新上传")
)

// StoragePolicy 是上传准入策略（双模式分叉点，见 ARCHITECTURE §5）。
// 内核默认 = 不设总配额（仅单文件 + 磁盘护栏）；SaaS 注入按套餐硬配额，本片不写。
type StoragePolicy interface {
	// AllowUpload 在接收新上传前调用，sizeBytes 为待写入字节数。
	AllowUpload(sizeBytes int64) error
}

// DefaultPolicy = 自部署默认：不限总量，仅护栏（单文件上限 + 磁盘最小可用）。
type DefaultPolicy struct {
	MaxFileBytes int64                          // 单文件上限
	MinFreeBytes uint64                         // 磁盘最小可用，低于则停新上传
	mediaRoot    string                         // 用于查磁盘可用
	freeFn       func(string) (uint64, error)   // 可注入，便于测试
}

// NewDefaultPolicy 构造默认策略。
func NewDefaultPolicy(mediaRoot string, maxFileBytes int64, minFreeBytes uint64) *DefaultPolicy {
	return &DefaultPolicy{
		MaxFileBytes: maxFileBytes,
		MinFreeBytes: minFreeBytes,
		mediaRoot:    mediaRoot,
		freeFn:       freeBytes,
	}
}

// AllowUpload 校验单文件大小与磁盘可用（优雅降级）。
func (p *DefaultPolicy) AllowUpload(sizeBytes int64) error {
	if p.MaxFileBytes > 0 && sizeBytes > p.MaxFileBytes {
		return ErrFileTooLarge
	}
	free, err := p.freeFn(p.mediaRoot)
	if err != nil {
		// 查不到可用空间时不冒进放行，按保守可用处理：放行（避免误伤），但记录由上层日志。
		return nil
	}
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	// 需为本次写入 + 缓冲留出余量。
	if free < p.MinFreeBytes+uint64(sizeBytes) { //nolint:gosec // 上面已保证 sizeBytes 非负
		return ErrDiskFull
	}
	return nil
}
