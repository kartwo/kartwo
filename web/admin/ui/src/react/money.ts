// 金额转换 / Exact Money Conversion
// 功能：两位小数金额与安全整数分互转，拒绝精度损失与溢出
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:01:04

export function moneyToCents(value: string): number {
  const text = value.trim()
  if (text.length > 24 || !/^\d+(?:\.\d{1,2})?$/.test(text)) throw new Error('金额须为非负数，最多两位小数')
  const [whole = '', fraction = ''] = text.split('.')
  const cents = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0'))
  if (cents > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('金额超出可精确处理的范围')
  return Number(cents)
}
export function centsToMoney(value: number): string {
  if (!Number.isSafeInteger(value) || value < 0) throw new Error('金额数据无效')
  const cents = BigInt(value)
  return `${cents / 100n}.${String(cents % 100n).padStart(2, '0')}`
}
