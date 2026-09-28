// 客服留言服务 / Contact Message Service
// 功能：访客留言、店主配置、后台状态管理与到期匿名化
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 14:00:00
package contact

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/settings"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

var (
	ErrDisabled  = errors.New("contact: 留言未开启")
	ErrInvalid   = errors.New("contact: 留言内容不合法")
	ErrNotFound  = errors.New("contact: 留言不存在")
	ErrUnchanged = errors.New("contact: 留言状态未变化")
)

type Config struct {
	Enabled         bool `json:"enabled"`
	RetentionMonths int  `json:"retention_months"`
	IPRetentionDays int  `json:"ip_retention_days"`
}

func DefaultConfig() Config { return Config{RetentionMonths: 12, IPRetentionDays: 90} }

type Input struct {
	Name     string
	Email    string
	Topic    string
	Body     string
	OrderRef string
	IP       string
	Country  string
}

type Message struct {
	PublicID         string `json:"public_id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Topic            string `json:"topic"`
	Body             string `json:"body"`
	OrderRef         string `json:"order_ref"`
	IP               string `json:"ip_address"`
	Country          string `json:"country_code"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	ClosedAt         string `json:"closed_at"`
	AnonymizedAt     string `json:"anonymized_at"`
	ProcessingByID   string `json:"processing_by_id"`
	ProcessingByName string `json:"processing_by_name"`
	ProcessingAt     string `json:"processing_at"`
	ClosedByID       string `json:"closed_by_id"`
	ClosedByName     string `json:"closed_by_name"`
}

// Actor 仅由后台已鉴权会话构造；用户名保存为操作时快照，账号改名不改历史。
type Actor struct {
	PublicID string
	Username string
}

type Service struct {
	db       *sql.DB
	settings *settings.Service
}

func New(db *sql.DB, settingSvc *settings.Service) *Service {
	return &Service{db: db, settings: settingSvc}
}

func (s *Service) Config(ctx context.Context) (Config, error) {
	c := DefaultConfig()
	for _, field := range []struct {
		key string
		set func(string) error
	}{
		{"contact.enabled", func(v string) error { c.Enabled = v == "1"; return nil }},
		{"contact.retention_months", func(v string) error { var err error; c.RetentionMonths, err = strconv.Atoi(v); return err }},
		{"contact.ip_retention_days", func(v string) error { var err error; c.IPRetentionDays, err = strconv.Atoi(v); return err }},
	} {
		v, err := s.settings.Get(ctx, field.key)
		if errors.Is(err, settings.ErrNotFound) {
			continue
		}
		if err != nil {
			return Config{}, err
		}
		if err := field.set(v); err != nil {
			return Config{}, err
		}
	}
	if !validConfig(c) {
		return Config{}, ErrInvalid
	}
	return c, nil
}

func validConfig(c Config) bool {
	return c.RetentionMonths >= 1 && c.RetentionMonths <= 24 && c.IPRetentionDays >= 7 && c.IPRetentionDays <= 90
}

func (s *Service) SaveConfig(ctx context.Context, c Config) error {
	if !validConfig(c) {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	values := []struct{ key, value string }{
		{"contact.enabled", "0"},
		{"contact.retention_months", fmt.Sprint(c.RetentionMonths)},
		{"contact.ip_retention_days", fmt.Sprint(c.IPRetentionDays)},
	}
	if c.Enabled {
		values[0].value = "1"
	}
	for _, item := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO setting (key,value,encrypted) VALUES (?,?,0) ON CONFLICT(key) DO UPDATE SET value=excluded.value, encrypted=0, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, item.key, item.value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validText(s string, min, max int, multiline bool) bool {
	n := len([]rune(s))
	if n < min || n > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}

func normalize(in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	in.Topic = strings.TrimSpace(in.Topic)
	in.Body = strings.TrimSpace(in.Body)
	in.OrderRef = strings.TrimSpace(in.OrderRef)
	if !validText(in.Name, 1, 80, false) || !validText(in.Body, 10, 4000, true) || !validText(in.OrderRef, 0, 60, false) || len(in.Email) > 254 {
		return ErrInvalid
	}
	addr, err := mail.ParseAddress(in.Email)
	if err != nil || addr.Address != in.Email || strings.ContainsAny(in.Email, "\r\n") {
		return ErrInvalid
	}
	switch in.Topic {
	case "product", "shipping", "order", "returns", "other":
	default:
		return ErrInvalid
	}
	return nil
}

func (s *Service) Submit(ctx context.Context, in Input) (string, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return "", err
	}
	if !c.Enabled {
		return "", ErrDisabled
	}
	if err := normalize(&in); err != nil {
		return "", err
	}
	id := uuid.Must(uuid.NewV7()).String()
	_, err = s.db.ExecContext(ctx, `INSERT INTO contact_message (public_id,name,email,topic,body,order_ref,ip_address,country_code) VALUES (?,?,?,?,?,?,?,?)`, id, in.Name, in.Email, in.Topic, in.Body, in.OrderRef, in.IP, in.Country)
	return id, err
}

func scanMessage(rows interface{ Scan(...any) error }) (Message, error) {
	var m Message
	err := rows.Scan(&m.PublicID, &m.Name, &m.Email, &m.Topic, &m.Body, &m.OrderRef, &m.IP, &m.Country, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.ClosedAt, &m.AnonymizedAt, &m.ProcessingByID, &m.ProcessingByName, &m.ProcessingAt, &m.ClosedByID, &m.ClosedByName)
	return m, err
}

const columns = `public_id,name,email,topic,body,order_ref,ip_address,country_code,status,created_at,updated_at,COALESCE(closed_at,''),COALESCE(anonymized_at,''),processing_by_id,processing_by_name,COALESCE(processing_at,''),closed_by_id,closed_by_name`

func (s *Service) List(ctx context.Context, status string, limit int) ([]Message, error) {
	if status != "" && status != "new" && status != "processing" && status != "closed" {
		return nil, ErrInvalid
	}
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM contact_message WHERE (?='' OR status=?) ORDER BY created_at DESC,id DESC LIMIT ?`, status, status, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]Message, 0)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, publicID string) (Message, error) {
	m, err := scanMessage(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM contact_message WHERE public_id=?`, publicID))
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	return m, err
}

func (s *Service) SetStatus(ctx context.Context, publicID, status string, actor Actor) error {
	if (status != "new" && status != "processing" && status != "closed") || actor.PublicID == "" || actor.Username == "" {
		return ErrInvalid
	}
	var query string
	var args []any
	switch status {
	case "processing":
		query = `UPDATE contact_message SET status='processing',processing_by_id=?,processing_by_name=?,processing_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND anonymized_at IS NULL AND status<>'processing'`
		args = []any{actor.PublicID, actor.Username, publicID}
	case "closed":
		query = `UPDATE contact_message SET status='closed',closed_by_id=?,closed_by_name=?,closed_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND anonymized_at IS NULL AND status<>'closed'`
		args = []any{actor.PublicID, actor.Username, publicID}
	case "new":
		query = `UPDATE contact_message SET status='new',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND anonymized_at IS NULL AND status<>'new'`
		args = []any{publicID}
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		var current string
		var anonymized sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT status,anonymized_at FROM contact_message WHERE public_id=?`, publicID).Scan(&current, &anonymized)
		if err == nil && !anonymized.Valid && current == status {
			return ErrUnchanged
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return ErrNotFound
	}
	return nil
}

// AnonymizeDue 保留工单元数据，不删除记录；先短期去 IP，再按结案时间去个人内容。
func (s *Service) AnonymizeDue(ctx context.Context, now time.Time) error {
	c, err := s.Config(ctx)
	if err != nil {
		return err
	}
	ipCutoff := now.UTC().AddDate(0, 0, -c.IPRetentionDays).Format(timeLayout)
	if _, err := s.db.ExecContext(ctx, `UPDATE contact_message SET ip_address='',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE ip_address<>'' AND created_at<?`, ipCutoff); err != nil {
		return err
	}
	messageCutoff := now.UTC().AddDate(0, -c.RetentionMonths, 0).Format(timeLayout)
	_, err = s.db.ExecContext(ctx, `UPDATE contact_message SET name='',email='',body='',order_ref='',ip_address='',country_code='',anonymized_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE status='closed' AND anonymized_at IS NULL AND closed_at<?`, messageCutoff)
	return err
}
