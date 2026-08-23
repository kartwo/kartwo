// 审计日志服务 / Audit Log Service
// 功能：以只追加方式记录并查询后台关键操作，不保存口令、密钥、会话令牌或请求正文
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-24 00:20:00
package audit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

const defaultListLimit int64 = 100

// Event 是对后台管理动作的最小可追溯记录。
type Event struct {
	PublicID       string
	Action         string
	TargetType     string
	TargetPublicID string
	CreatedAt      string
	AdminPublicID  string
	AdminUsername  string
}

// Service 提供审计事件的追加和只读查询。
type Service struct{ q *sqlcgen.Queries }

// New 构建审计服务。
func New(db *sql.DB) *Service { return &Service{q: sqlcgen.New(db)} }

// Record 追加一条成功管理动作的最小事件。调用者只传稳定对象标识，绝不传请求正文或敏感数据。
func (s *Service) Record(ctx context.Context, adminID int64, action, targetType, targetPublicID string) error {
	if adminID <= 0 || strings.TrimSpace(action) == "" {
		return fmt.Errorf("audit: 管理员与动作不能为空")
	}
	if len(action) > 100 || len(targetType) > 100 || len(targetPublicID) > 200 {
		return fmt.Errorf("audit: 事件字段超长")
	}
	publicID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("audit: 生成事件标识失败: %w", err)
	}
	if err := s.q.CreateAuditEvent(ctx, sqlcgen.CreateAuditEventParams{
		PublicID: publicID.String(), AdminID: adminID, Action: action,
		TargetType: targetType, TargetPublicID: targetPublicID,
	}); err != nil {
		return fmt.Errorf("audit: 写入事件失败: %w", err)
	}
	return nil
}

// List 返回最近事件，limit 非法时回退为默认安全上限。
func (s *Service) List(ctx context.Context, limit int64) ([]Event, error) {
	if limit <= 0 || limit > defaultListLimit {
		limit = defaultListLimit
	}
	rows, err := s.q.ListAuditEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: 查询事件失败: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, Event{
			PublicID: row.PublicID, Action: row.Action, TargetType: row.TargetType,
			TargetPublicID: row.TargetPublicID, CreatedAt: row.CreatedAt,
			AdminPublicID: row.AdminPublicID, AdminUsername: row.AdminUsername,
		})
	}
	return out, nil
}
