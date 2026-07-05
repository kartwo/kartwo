// 配置加密测试 / Config Encryption Tests
// 功能：AES-GCM 加解密往返、错误 KEK/损坏密文拒绝（敏感设置安全）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-19 21:22:05
package auth

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return k
}

func TestEncryptRoundTrip(t *testing.T) {
	k := key(t)
	pt := []byte("sk_test_秘密支付密钥-123")
	ct, err := Encrypt(k, pt)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if bytes.Contains([]byte(ct), pt) {
		t.Fatal("密文不应包含明文")
	}
	got, err := Decrypt(k, ct)
	if err != nil || !bytes.Equal(got, pt) {
		t.Fatalf("解密不符: %v / %q", err, got)
	}
}

func TestEncryptUniquePerCall(t *testing.T) {
	k := key(t)
	a, _ := Encrypt(k, []byte("same"))
	b, _ := Encrypt(k, []byte("same"))
	if a == b {
		t.Fatal("随机 nonce 应使两次密文不同")
	}
}

func TestDecryptWrongKeyOrCorrupt(t *testing.T) {
	k1, k2 := key(t), key(t)
	ct, _ := Encrypt(k1, []byte("secret"))
	if _, err := Decrypt(k2, ct); err != ErrDecrypt {
		t.Fatalf("错误 KEK 应 ErrDecrypt: %v", err)
	}
	if _, err := Decrypt(k1, ct+"AA"); err != ErrDecrypt {
		t.Fatalf("损坏密文应 ErrDecrypt: %v", err)
	}
	if _, err := Decrypt(k1, "!!!"); err != ErrDecrypt {
		t.Fatalf("非法 base64 应 ErrDecrypt: %v", err)
	}
}
