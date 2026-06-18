// 鉴权原语测试 / Auth Primitives Tests
// 功能：验证 argon2id 口令哈希/校验与 KEK 派生（核心安全逻辑必须单测）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 23:18:17
package auth

import (
	"bytes"
	"testing"
)

func TestHashVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if hash == "correct horse battery" {
		t.Fatal("哈希不应等于明文")
	}

	ok, err := VerifyPassword(hash, "correct horse battery")
	if err != nil || !ok {
		t.Fatalf("正确口令应校验通过: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(hash, "wrong")
	if err != nil {
		t.Fatalf("校验错误口令不应报错: %v", err)
	}
	if ok {
		t.Fatal("错误口令不应通过")
	}
}

func TestHashUniquePerCall(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("同口令两次哈希应因随机盐而不同")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "plain", "$argon2id$bad", "$bcrypt$v=19$m=1,t=1,p=1$a$b"} {
		if _, err := VerifyPassword(bad, "x"); err == nil {
			t.Fatalf("非法编码 %q 应报错", bad)
		}
	}
}

func TestKeySourceDerivation(t *testing.T) {
	salt1, _ := NewKEKSalt()
	salt2, _ := NewKEKSalt()
	if bytes.Equal(salt1, salt2) {
		t.Fatal("两次生成的盐应不同")
	}

	ks := NewMasterPasswordKeySource(salt1)
	k1, err := ks.DeriveKey("master-pass")
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	if len(k1) != argonKeyLen {
		t.Fatalf("KEK 长度 = %d，期望 %d", len(k1), argonKeyLen)
	}

	// 同口令+同盐恒定。
	k1b, _ := ks.DeriveKey("master-pass")
	if !bytes.Equal(k1, k1b) {
		t.Fatal("同口令+同盐应派生相同 KEK")
	}
	// 不同口令 → 不同 KEK。
	k2, _ := ks.DeriveKey("other-pass")
	if bytes.Equal(k1, k2) {
		t.Fatal("不同口令应派生不同 KEK")
	}
	// 不同盐 → 不同 KEK。
	k3, _ := NewMasterPasswordKeySource(salt2).DeriveKey("master-pass")
	if bytes.Equal(k1, k3) {
		t.Fatal("不同盐应派生不同 KEK")
	}
}
