// 支付切换提示测试 / Checkout Switch Messages
// 功能：验证付款处理中和已停用链接仅公开固定提示，不泄漏网关响应
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:58:12
package storefront

import (
	"fmt"
	"github.com/kartwo/kartwo/internal/payment"
	"strings"
	"testing"
)

func TestPaymentSwitchErrorMessages(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{payment.ErrPaymentProcessing, "processing"}, {payment.ErrCheckoutReplaced, "replaced"}, {payment.ErrPaymentInProgress, "busy"}, {payment.ErrPaymentConfigChanged, "config"}} {
		code := paymentErrorCode(fmt.Errorf("private-provider-response: %w", tc.err))
		if code != tc.code {
			t.Fatalf("code %s want %s", code, tc.code)
		}
		if msg := paymentErrorMessage(code); msg == "" || strings.Contains(msg, "private-provider-response") {
			t.Fatalf("unsafe message %q", msg)
		}
	}
}
