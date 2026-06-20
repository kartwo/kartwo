// Stripe Webhook 验签 / Stripe Signature Verification
// 功能：对【原始请求字节】做 HMAC-SHA256 验签 + 时间戳容差防重放；纯函数，可独立单测
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// defaultTolerance 时间戳容差（与 Stripe 官方默认一致）。
const defaultTolerance = 5 * time.Minute

// verifyStripeSignature 校验 Stripe-Signature 头。
// 必须对【未经任何中间件预读/改写的原始字节 payload】计算 HMAC，否则签名必然不符。
// scheme：signed_payload = "{t}.{payload}"，HMAC-SHA256(key=whsec 全字符串)，与头中任一 v1 常量时间比对。
func verifyStripeSignature(payload []byte, header, secret string, tolerance time.Duration, now time.Time) error {
	ts, sigs := parseSigHeader(header)
	if ts == "" || len(sigs) == 0 {
		return ErrSigFormat
	}
	tsec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrSigFormat
	}
	// 时间戳容差（双向，兼防重放与时钟前漂）。
	delta := now.Unix() - tsec
	if delta < 0 {
		delta = -delta
	}
	if delta > int64(tolerance/time.Second) {
		return ErrSigExpired
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := mac.Sum(nil)

	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err != nil {
			continue
		}
		if hmac.Equal(expected, got) {
			return nil
		}
	}
	return ErrSigMismatch
}

// parseSigHeader 解析 "t=...,v1=...,v1=..." 形式，返回时间戳与全部 v1 签名。
func parseSigHeader(header string) (ts string, v1 []string) {
	for part := range strings.SplitSeq(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			v1 = append(v1, kv[1])
		}
	}
	return ts, v1
}
