// 接口值校验 / API Value Validation
// 功能：在 JSON 边界校验对象与字段，拒绝缺失或未知状态
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:34:00

export function record(data: unknown): Record<string, unknown> {
  if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error('配置响应格式不正确，请重试。')
  return data as Record<string, unknown>
}
export function string(data: Record<string, unknown>, key: string): string {
  const value = data[key]
  if (typeof value !== 'string') throw new Error('配置响应缺少必要字段，请重试。')
  return value
}
export function bool(data: Record<string, unknown>, key: string): boolean {
  const value = data[key]
  if (typeof value !== 'boolean') throw new Error('配置响应缺少必要状态，请重试。')
  return value
}
export function choice<T extends string>(data: Record<string, unknown>, key: string, values: readonly T[]): T {
  const value = string(data, key)
  if (!values.includes(value as T)) throw new Error('配置状态无法识别，请重试。')
  return value as T
}
export function integer(data: Record<string, unknown>, key: string, min: number, max: number): number {
  const value = data[key]
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < min || value > max) throw new Error('配置数值无效，请重试。')
  return value
}
