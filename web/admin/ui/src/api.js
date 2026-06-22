// API 客户端 / API Client
// 功能：统一 fetch 封装，自动带 cookie 与 CSRF 头，401 抛出供上层跳登录
// 作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-06-18 10:20:00

function csrfToken() {
  const m = document.cookie.match(/(?:^|;\s*)kartwo_csrf=([^;]+)/)
  return m ? decodeURIComponent(m[1]) : ''
}

export class APIError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

async function request(method, path, body, isForm) {
  const headers = {}
  const opts = { method, credentials: 'same-origin', headers }
  if (!['GET', 'HEAD'].includes(method)) {
    headers['X-CSRF-Token'] = csrfToken()
  }
  if (body !== undefined && !isForm) {
    headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  } else if (isForm) {
    opts.body = body
  }
  const res = await fetch('/admin/api' + path, opts)
  let data = null
  const text = await res.text()
  if (text) {
    try { data = JSON.parse(text) } catch { data = { raw: text } }
  }
  if (!res.ok) {
    throw new APIError(res.status, (data && data.error) || ('HTTP ' + res.status))
  }
  return data
}

export const api = {
  status: () => request('GET', '/status'),
  me: () => request('GET', '/me'),
  setup: (username, password) => request('POST', '/setup', { username, password }),
  login: (username, password) => request('POST', '/login', { username, password }),
  logout: () => request('POST', '/logout'),

  listProducts: () => request('GET', '/products'),
  getProduct: (id) => request('GET', '/products/' + id),
  createProduct: (payload) => request('POST', '/products', payload),
  updateProduct: (id, payload) => request('PATCH', '/products/' + id, payload),
  deleteProduct: (id) => request('DELETE', '/products/' + id),
  setInventory: (variantId, quantity) => request('PATCH', '/variants/' + variantId + '/inventory', { quantity }),

  markets: () => request('GET', '/markets'),
  getMarket: () => request('GET', '/settings/market'),
  setMarket: (code) => request('PUT', '/settings/market', { code }),

  getPayment: () => request('GET', '/settings/payment'),
  setPayment: (payload) => request('PUT', '/settings/payment', payload),

  listOrders: () => request('GET', '/orders'),
  getOrder: (id) => request('GET', '/orders/' + id),
  refundOrder: (id) => request('POST', '/orders/' + id + '/refund'),

  listMedia: (productId) => request('GET', '/products/' + productId + '/media'),
  deleteMedia: (mediaId) => request('DELETE', '/media/' + mediaId),
  uploadMedia: (productId, file) => {
    const fd = new FormData()
    fd.append('file', file)
    return request('POST', '/products/' + productId + '/media', fd, true)
  },
}
