// 配置加密 / Config Encryption
// 功能：用 KEK（主口令派生）对敏感设置做 AES-256-GCM 加解密；密文自带随机 nonce
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-19 21:22:05
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrDecrypt 表示密文非法或 KEK 不匹配（解密失败）。
var ErrDecrypt = errors.New("auth: 解密失败（密文损坏或主口令不符）")

// Encrypt 用 32 字节 KEK 对明文做 AES-256-GCM 加密，返回 base64(nonce||ciphertext)。
func Encrypt(kek, plaintext []byte) (string, error) {
	gcm, err := newGCM(kek)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("auth: 生成 nonce 失败: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解开 Encrypt 的产物；KEK 不符或密文损坏返回 ErrDecrypt。
func Decrypt(kek []byte, encoded string) ([]byte, error) {
	gcm, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrDecrypt
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, ErrDecrypt
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

func newGCM(kek []byte) (cipher.AEAD, error) {
	if len(kek) != 32 {
		return nil, fmt.Errorf("auth: KEK 长度应为 32，得 %d", len(kek))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("auth: 建 AES 失败: %w", err)
	}
	return cipher.NewGCM(block)
}
