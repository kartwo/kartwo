// 加密密钥来源 / Encryption Key Source
// 功能：由主口令派生配置加密密钥(KEK)；双模式分叉点——默认=主口令派生，SaaS 托管后置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package auth

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// KeySource 是配置加密密钥(KEK)的来源接口（双模式分叉点，见 ARCHITECTURE §15）。
// 内核默认实现 = 主口令派生；SaaS 平台托管为 v2+ 的另一实现，本片不写。
type KeySource interface {
	// DeriveKey 由主口令派生 32 字节 KEK；同口令+同盐恒定。KEK 绝不落明文。
	DeriveKey(masterPassword string) ([]byte, error)
}

// MasterPasswordKeySource 用存库的盐 + argon2id 由主口令派生 KEK。
// 盐非密钥、可明文存（落 meta 表）；真正的 KEK 只在内存中、随用随派生。
type MasterPasswordKeySource struct {
	salt []byte // KEK 专用盐（与登录口令哈希用的盐不同，隔离两用途）
}

// NewMasterPasswordKeySource 用已存盐构造。
func NewMasterPasswordKeySource(salt []byte) *MasterPasswordKeySource {
	return &MasterPasswordKeySource{salt: salt}
}

// NewKEKSalt 生成一条新的 KEK 盐（初始化时调用一次并持久化）。
func NewKEKSalt() ([]byte, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("auth: 生成 KEK 盐失败: %w", err)
	}
	return salt, nil
}

// DeriveKey 由主口令 + 盐派生 KEK（参数与口令哈希同档，但盐不同）。
func (s *MasterPasswordKeySource) DeriveKey(masterPassword string) ([]byte, error) {
	if len(s.salt) == 0 {
		return nil, fmt.Errorf("auth: KEK 盐为空，未初始化")
	}
	return argon2.IDKey([]byte(masterPassword), s.salt, argonTime, argonMemKiB, argonThreads, argonKeyLen), nil
}
