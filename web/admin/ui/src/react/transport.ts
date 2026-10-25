// 后台请求传输 / Admin Request Transport
// 功能：统一同源 Cookie、CSRF、JSON、上传与错误响应处理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:10:05

export class APIError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '操作失败，请重试。'
}

function csrfToken(): string {
  const value = document.cookie.match(/(?:^|;\s*)kartwo_csrf=([^;]+)/)?.[1]
  if (!value) return ''
  try { return decodeURIComponent(value) } catch { return '' }
}

export type Method = 'GET' | 'HEAD' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
export type JSONValue = null | boolean | number | string | JSONValue[] | { [key: string]: JSONValue | undefined }

export function responseError(data: unknown, fallback: string): string {
  if (typeof data === 'object' && data !== null && 'error' in data && typeof data.error === 'string') {
    return data.error || fallback
  }
  return fallback
}

export async function request<T = unknown>(method: Method, path: string, body?: JSONValue | FormData, signal?: AbortSignal): Promise<T> {
  // Paths are application-owned relative API paths, never external URLs.
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\')) throw new Error('无效的 API 路径')
  const headers: Record<string, string> = { Accept: 'application/json' }
  const options: RequestInit = { method, credentials: 'same-origin', cache: 'no-store', headers, signal }
  if (method !== 'GET' && method !== 'HEAD') headers['X-CSRF-Token'] = csrfToken()
  if (body instanceof FormData) options.body = body
  else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    options.body = JSON.stringify(body)
  }
  const response = await fetch('/admin/api' + path, options)
  const text = await response.text()
  let data: unknown = null
  if (text) {
    try { data = JSON.parse(text) as unknown }
    catch {
      if (response.ok) throw new APIError(response.status, '服务器响应格式不正确，请重试。')
    }
  }
  if (!response.ok) throw new APIError(response.status, responseError(data, `HTTP ${response.status}`))
  return data as T
}
