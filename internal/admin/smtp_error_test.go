// 邮件测试错误保护 / SMTP Test Error Protection
// 功能：验证未知投递不误报为确定失败，SMTP 返回内容不泄漏到后台响应
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 14:27:16
package admin

import (
	"bytes"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/kartwo/kartwo/internal/mail"
)

func TestSMTPTestErrorsAreSafeAndRespectUncertainty(t *testing.T) {
	const private = "private-recipient@example.invalid fixture-smtp-password private-message-text"
	for _, test := range []struct {
		name, message string
		err           error
		code          int
	}{
		{"unknown", "投递结果未知", fmt.Errorf("wrapped: %w: %s", mail.ErrDeliveryUnknown, private), 502},
		{"invalid", "地址格式", fmt.Errorf("wrapped: %w: %s", mail.ErrInvalidMessage, private), 400},
		{"rejected", "SMTP 测试未成功", errors.New(private), 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeSMTPTestError(response, test.err)
			if response.Code != test.code || !bytes.Contains(response.Body.Bytes(), []byte(test.message)) {
				t.Fatalf("incorrect SMTP feedback: code=%d", response.Code)
			}
			for _, secret := range [][]byte{[]byte("private-recipient"), []byte("fixture-smtp-password"), []byte("private-message-text")} {
				if bytes.Contains(response.Body.Bytes(), secret) {
					t.Fatal("SMTP response leaked transport details")
				}
			}
		})
	}
}
