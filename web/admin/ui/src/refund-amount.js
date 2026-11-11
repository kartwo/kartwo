// 退款金额 / Refund Amount
// 功能：将商家输入的两位小数金额精确转换为整数分，拒绝溢出及隐式四舍五入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 22:40:00
export function refundCents(value) {
  const text = String(value).trim()
  if (text.length > 24) return null
  if (!/^\d+(?:\.\d{1,2})?$/.test(text)) return null
  const [whole, fraction = ''] = text.split('.')
  const cents = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0'))
  return cents > 0n && cents <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(cents) : null
}
