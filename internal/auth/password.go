// 口令哈希 / Password Hashing
// 功能：argon2id 口令哈希与常数时间校验（PHC 编码自带参数，便于将来调参）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id 参数（决策：1C1G 友好档，见 DECISIONS）。
const (
	argonMemKiB  = 64 * 1024 // 64 MiB
	argonTime    = 3
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrInvalidHash 表示编码串格式非法或不受支持。
var ErrInvalidHash = errors.New("auth: 非法或不支持的口令哈希编码")

// HashPassword 用 argon2id 哈希口令，返回自带参数的 PHC 编码串：
// $argon2id$v=19$m=65536,t=3,p=1$<b64salt>$<b64hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: 生成盐失败: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword 以编码串中的参数重算并常数时间比对。
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrInvalidHash
	}
	var mem, t uint32
	var p uint8 // 直接解析为 uint8，避免越界转换
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}
	// 哈希长度做合理边界校验（同时让转换显式安全）。
	if len(want) < 16 || len(want) > 64 {
		return false, ErrInvalidHash
	}
	keyLen := uint32(len(want)) //nolint:gosec // 上面已限定 16..64，转换安全
	got := argon2.IDKey([]byte(password), salt, t, mem, p, keyLen)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
