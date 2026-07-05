// 设置服务 / Settings Service
// 功能：键值设置读写（明文/KEK 加密）、主攻市场选择与派生（货币/语言/RTL）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-19 21:22:05
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kartwo/kartwo/internal/auth"
	"github.com/kartwo/kartwo/internal/market"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

const keyMarketCode = "market.code"

var (
	// ErrNotFound 设置项不存在。
	ErrNotFound = errors.New("settings: 不存在")
	// ErrMarketUnavailable 市场未点亮（即将上线）。
	ErrMarketUnavailable = errors.New("settings: 该市场即将上线，暂不可选")
	// ErrNotEncrypted 期望加密项但存的是明文（或反之）。
	ErrNotEncrypted = errors.New("settings: 该项不是加密存储")
)

// Service 承载设置读写。
type Service struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// New 构造设置服务。
func New(db *sql.DB) *Service { return &Service{db: db, q: sqlcgen.New(db)} }

// Get 读取明文设置项。不存在返回 ErrNotFound。
func (s *Service) Get(ctx context.Context, key string) (string, error) {
	row, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", fmt.Errorf("settings: 读取失败: %w", err)
	}
	return row.Value, nil
}

// SetPlain 写入明文设置项。
func (s *Service) SetPlain(ctx context.Context, key, value string) error {
	return s.q.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: key, Value: value, Encrypted: 0})
}

// SetEncrypted 用 KEK 加密后写入敏感设置项（如支付密钥）。
func (s *Service) SetEncrypted(ctx context.Context, key string, plaintext, kek []byte) error {
	enc, err := auth.Encrypt(kek, plaintext)
	if err != nil {
		return err
	}
	return s.q.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: key, Value: enc, Encrypted: 1})
}

// GetEncrypted 用 KEK 解密读取敏感设置项。不存在返回 ErrNotFound。
func (s *Service) GetEncrypted(ctx context.Context, key string, kek []byte) ([]byte, error) {
	row, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("settings: 读取失败: %w", err)
	}
	if row.Encrypted == 0 {
		return nil, ErrNotEncrypted
	}
	return auth.Decrypt(kek, row.Value)
}

// MarketCode 返回当前主攻市场代码（未设则默认美国）。
func (s *Service) MarketCode(ctx context.Context) string {
	v, err := s.Get(ctx, keyMarketCode)
	if err != nil || v == "" {
		return market.Default().Code
	}
	return v
}

// SetMarketCode 设置主攻市场（必须是已点亮市场）。
func (s *Service) SetMarketCode(ctx context.Context, code string) error {
	if !market.IsAvailable(code) {
		return ErrMarketUnavailable
	}
	return s.SetPlain(ctx, keyMarketCode, code)
}

// Market 返回当前主攻市场配置。
func (s *Service) Market(ctx context.Context) market.Market {
	m, ok := market.Lookup(s.MarketCode(ctx))
	if !ok {
		return market.Default()
	}
	return m
}

// Currency 返回当前市场货币（如 USD）。
func (s *Service) Currency(ctx context.Context) string { return s.Market(ctx).Currency }
