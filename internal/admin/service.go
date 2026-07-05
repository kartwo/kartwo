// Admin 鉴权服务 / Admin Auth Service
// 功能：首次初始化(建管理员+设主口令)、登录/登出、会话校验、主口令派生 KEK(内存持有)
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/auth"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// 会话有效期与 KEK 盐在 meta 表中的键名。
const (
	sessionTTL    = 7 * 24 * time.Hour
	metaKeyKEKalt = "security.kek_salt"
	timeLayout    = "2006-01-02T15:04:05.000Z" // 与迁移里 strftime 一致，便于字符串比较
)

var (
	// ErrAlreadyInitialized 表示已存在管理员，拒绝重复初始化。
	ErrAlreadyInitialized = errors.New("admin: 已初始化")
	// ErrInvalidCredentials 表示用户名或口令错误（不区分，防枚举）。
	ErrInvalidCredentials = errors.New("admin: 用户名或口令错误")
	// ErrUnauthorized 表示会话无效或过期。
	ErrUnauthorized = errors.New("admin: 未授权")
)

// PaymentKeys 是收款密钥内存缓存的生命周期钩子（由 internal/payment 实现）。
// 绑定 KEK 金库：登录解锁时 Unlock 载入、登出时 Lock 销毁；Status 供收款页判定来源(env|db)。
type PaymentKeys interface {
	Unlock(ctx context.Context, kek []byte) error
	Lock()
	Status() payment.CacheStatus
}

// Service 承载 Admin 鉴权逻辑。
type Service struct {
	db    *sql.DB
	q     *sqlcgen.Queries
	vault *kekVault    // 内存中持有已解锁会话的 KEK，绝不落盘
	keys  PaymentKeys // 可选：收款密钥缓存，随登录/登出解锁/销毁
}

// New 构建 Admin 服务。
func New(db *sql.DB) *Service {
	return &Service{db: db, q: sqlcgen.New(db), vault: newKEKVault()}
}

// SetPaymentKeys 注入收款密钥缓存钩子（main 装配时调用）。
func (s *Service) SetPaymentKeys(k PaymentKeys) { s.keys = k }

// ReloadKeys 用给定 KEK 立即重载收款密钥缓存（收款页改密钥后调用，使其即时生效）。
func (s *Service) ReloadKeys(ctx context.Context, kek []byte) error {
	if s.keys == nil {
		return nil
	}
	return s.keys.Unlock(ctx, kek)
}

// PaymentStatus 返回收款密钥来源状态（含 env|db）；keys 未注入时 ok=false。
func (s *Service) PaymentStatus() (payment.CacheStatus, bool) {
	if s.keys == nil {
		return payment.CacheStatus{}, false
	}
	return s.keys.Status(), true
}

// Session 为一次登录产生的会话凭据。
type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

// AuthContext 为已鉴权请求的身份。
type AuthContext struct {
	AdminID         int64
	Username        string
	AdminPublicID   string
	SessionToken    string
	CSRFToken       string
}

// IsInitialized 报告是否已建管理员。
func (s *Service) IsInitialized(ctx context.Context) (bool, error) {
	n, err := s.q.CountAdminUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("admin: 统计管理员失败: %w", err)
	}
	return n > 0, nil
}

// Initialize 首次初始化：建唯一管理员 + 生成并存 KEK 盐。已初始化则拒绝。
func (s *Service) Initialize(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("admin: 用户名与口令不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("admin: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	n, err := q.CountAdminUsers(ctx)
	if err != nil {
		return fmt.Errorf("admin: 统计管理员失败: %w", err)
	}
	if n > 0 {
		return ErrAlreadyInitialized
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := q.CreateAdminUser(ctx, sqlcgen.CreateAdminUserParams{
		PublicID: uuid.Must(uuid.NewV7()).String(), Username: username, PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("admin: 建管理员失败: %w", err)
	}

	salt, err := auth.NewKEKSalt()
	if err != nil {
		return err
	}
	if err := q.UpsertMeta(ctx, sqlcgen.UpsertMetaParams{
		Key: metaKeyKEKalt, Value: base64.RawStdEncoding.EncodeToString(salt),
	}); err != nil {
		return fmt.Errorf("admin: 存 KEK 盐失败: %w", err)
	}
	return tx.Commit()
}

// Login 校验口令，派生 KEK 并入内存金库，建会话。返回会话凭据。
func (s *Service) Login(ctx context.Context, username, password string) (*Session, error) {
	user, err := s.q.GetAdminUserByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	} else if err != nil {
		return nil, fmt.Errorf("admin: 取管理员失败: %w", err)
	}

	ok, err := auth.VerifyPassword(user.PasswordHash, password)
	if err != nil {
		return nil, fmt.Errorf("admin: 校验口令失败: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	// 口令正确 → 派生 KEK（证明可解锁配置），仅入内存金库。
	kek, err := s.deriveKEK(ctx, password)
	if err != nil {
		return nil, err
	}

	token, err := randToken()
	if err != nil {
		return nil, err
	}
	csrf, err := randToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(sessionTTL)
	if err := s.q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		Token: token, AdminID: user.ID, CsrfToken: csrf, ExpiresAt: expires.Format(timeLayout),
	}); err != nil {
		return nil, fmt.Errorf("admin: 建会话失败: %w", err)
	}
	s.vault.put(token, kek)
	// 解锁收款密钥缓存（绑定 KEK 金库生命周期）。失败不阻断登录——收款页/诊断会提示。
	if s.keys != nil {
		_ = s.keys.Unlock(ctx, kek)
	}
	return &Session{Token: token, CSRFToken: csrf, ExpiresAt: expires}, nil
}

// Authenticate 校验会话 token（未过期），返回身份。
func (s *Service) Authenticate(ctx context.Context, token string) (*AuthContext, error) {
	if token == "" {
		return nil, ErrUnauthorized
	}
	row, err := s.q.GetSessionByToken(ctx, sqlcgen.GetSessionByTokenParams{
		Token: token, ExpiresAt: time.Now().UTC().Format(timeLayout),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthorized
	} else if err != nil {
		return nil, fmt.Errorf("admin: 取会话失败: %w", err)
	}
	return &AuthContext{
		AdminID: row.AdminID, Username: row.Username, AdminPublicID: row.PublicID,
		SessionToken: row.Token, CSRFToken: row.CsrfToken,
	}, nil
}

// Logout 删除会话并清内存 KEK；同时销毁收款密钥缓存（退出即销毁）。
func (s *Service) Logout(ctx context.Context, token string) error {
	s.vault.delete(token)
	if s.keys != nil {
		s.keys.Lock()
	}
	if err := s.q.DeleteSession(ctx, token); err != nil {
		return fmt.Errorf("admin: 删会话失败: %w", err)
	}
	return nil
}

// deriveKEK 读存库的盐，由主口令派生 KEK。
func (s *Service) deriveKEK(ctx context.Context, password string) ([]byte, error) {
	m, err := s.q.GetMeta(ctx, metaKeyKEKalt)
	if err != nil {
		return nil, fmt.Errorf("admin: 取 KEK 盐失败: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(m.Value)
	if err != nil {
		return nil, fmt.Errorf("admin: 解析 KEK 盐失败: %w", err)
	}
	return auth.NewMasterPasswordKeySource(salt).DeriveKey(password)
}

func randToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("admin: 生成随机 token 失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// kekVault 内存持有 token→KEK 映射，绝不落盘。
type kekVault struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func newKEKVault() *kekVault { return &kekVault{m: make(map[string][]byte)} }

func (v *kekVault) put(token string, kek []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[token] = kek
}

func (v *kekVault) delete(token string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, token)
}

// Key 返回会话已解锁的 KEK（M3 起消费；未解锁返回 false）。
func (s *Service) Key(token string) ([]byte, bool) {
	s.vault.mu.RLock()
	defer s.vault.mu.RUnlock()
	k, ok := s.vault.m[token]
	return k, ok
}
