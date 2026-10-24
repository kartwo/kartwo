// 退款金额测试 / Refund Amount Tests
// 功能：验证金额分的准确边界与非法输入拒绝
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 22:40:00
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { refundCents } from './refund-amount.js'

test('退款金额保持整数分，不接受隐式舍入或科学计数', () => {
  for (const [text, cents] of [['0.01', 1], ['10.29', 1029], [' 12.3 ', 1230], ['25', 2500], ['90071992547409.91', Number.MAX_SAFE_INTEGER]]) {
    assert.equal(refundCents(text), cents)
  }
  for (const text of ['', '0', '-1', '0.001', '1e2', '1,000', 'Infinity', '90071992547409.92', '25.']) {
    assert.equal(refundCents(text), null)
  }
})
