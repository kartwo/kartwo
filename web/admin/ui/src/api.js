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

// contentPagePayload 只发送内容页 API 明确定义的字段。表单中的 public_id、updated_at
// 属于界面状态，严格 JSON 解码器会正确拒绝它们，不能原样透传。
function contentPagePayload(page) {
  return {
    title: page.title,
    slug: page.slug,
    body_markdown: page.body_markdown,
    seo_description: page.seo_description,
    status: page.status,
  }
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
  setProductFeatured: (id, featured) => request('PATCH', '/products/' + id + '/featured', { featured }),
  listCategories: () => request('GET', '/categories'),
  createCategory: (payload) => request('POST', '/categories', payload),
  updateCategory: (id, payload) => request('PATCH', '/categories/' + id, payload),
  deleteCategory: (id) => request('DELETE', '/categories/' + id),
  listContentPages: () => request('GET', '/content-pages'),
  getContentPage: (id) => request('GET', '/content-pages/' + id),
  createContentPage: (payload) => request('POST', '/content-pages', contentPagePayload(payload)),
  updateContentPage: (id, payload) => request('PATCH', '/content-pages/' + id, contentPagePayload(payload)),
  deleteContentPage: (id) => request('DELETE', '/content-pages/' + id),
  generateFooterPages: (payload) => request('POST', '/content-pages/generate-footer', payload),
  setInventory: (variantId, quantity) => request('PATCH', '/variants/' + variantId + '/inventory', { quantity }),
  setPrice: (variantId, priceCents) => request('PATCH', '/variants/' + variantId + '/price', { price_cents: priceCents }),
  previewImportCSV: (file, format) => { const fd = new FormData(); fd.append('file', file); fd.append('format', format); return request('POST', '/imports/csv/preview', fd, true) },
  executeImport: (id) => request('POST', '/imports/' + id + '/execute'),
  getImport: (id) => request('GET', '/imports/' + id),

  getPayment: () => request('GET', '/settings/payment'),
  setPayment: (payload) => request('PUT', '/settings/payment', payload),
  testStripeConnection: () => request('POST', '/settings/payment/stripe/test'),
  getShop: () => request('GET', '/settings/shop'),
  setShop: (name) => request('PUT', '/settings/shop', { name }),
  uploadShopLogo: (file) => { const fd = new FormData(); fd.append('file', file); return request('POST', '/settings/shop/logo', fd, true) },
  deleteShopLogo: () => request('DELETE', '/settings/shop/logo'),
  getPolicyProfile: () => request('GET', '/settings/policy-profile'),
  setPolicyProfile: (payload) => request('PUT', '/settings/policy-profile', payload),
  listShippingCountries: () => request('GET', '/settings/shipping/countries'),
  saveShippingCountries: (countries) => request('PUT', '/settings/shipping/countries', { countries }),
  listShippingZones: () => request('GET', '/settings/shipping'),
  createShippingZone: (payload) => request('POST', '/settings/shipping', payload),
  saveDefaultShippingZone: (payload) => request('PUT', '/settings/shipping/default', payload),
  updateShippingZone: (id, payload) => request('PATCH', '/settings/shipping/' + id, payload),
  deleteShippingZone: (id) => request('DELETE', '/settings/shipping/' + id),
  getTranslation: () => request('GET', '/settings/translation'),
  setTranslation: (payload) => request('PUT', '/settings/translation', payload),
  translateText: (text) => request('POST', '/translation/text', { text }),
  wizardPayment: () => request('GET', '/wizard/payment'),
  wizardPaymentSkip: () => request('POST', '/wizard/payment/skip'),

  getDomain: () => request('GET', '/settings/domain'),
  setDomain: (domain) => request('PUT', '/settings/domain', { domain }),
  wizardDomain: () => request('GET', '/wizard/domain'),
  wizardDomainSkip: () => request('POST', '/wizard/domain/skip'),

  getSmtp: () => request('GET', '/settings/smtp'),
  setSmtp: (payload) => request('PUT', '/settings/smtp', payload),
  smtpTest: (to) => request('POST', '/smtp/test', { to }),
  wizardSmtp: () => request('GET', '/wizard/smtp'),
  wizardSmtpSkip: () => request('POST', '/wizard/smtp/skip'),

  dashboard: () => request('GET', '/dashboard'),
  diagnostics: () => request('GET', '/diagnostics'),
  getBackupSettings: () => request('GET', '/settings/backup'),
  setBackupSettings: (payload) => request('PUT', '/settings/backup', payload),
  testBackup: () => request('POST', '/settings/backup/test'),
  auditEvents: () => request('GET', '/audit-events'),

  listOrders: () => request('GET', '/orders'),
  exportOrdersCSV: async (filter = {}) => {
    const query = new URLSearchParams()
    if (filter.status) query.set('status', filter.status)
    if (filter.from) query.set('from', filter.from)
    if (filter.to) query.set('to', filter.to)
    const suffix = query.toString() ? '?' + query.toString() : ''
    const res = await fetch('/admin/api/orders/export' + suffix, { credentials: 'same-origin' })
    if (!res.ok) {
      let message = '导出订单失败'
      try { message = (await res.json()).error || message } catch (_) {}
      throw new APIError(res.status, message)
    }
    return res.blob()
  },
  getOrder: (id) => request('GET', '/orders/' + id),
  refundOrder: (id) => request('POST', '/orders/' + id + '/refund'),
  fulfillOrder: (id, payload) => request('POST', '/orders/' + id + '/fulfill', payload),

  listMedia: (productId) => request('GET', '/products/' + productId + '/media'),
  deleteMedia: (mediaId) => request('DELETE', '/media/' + mediaId),
  updateMediaAlt: (mediaId, altText) => request('PATCH', '/media/' + mediaId, { alt_text: altText }),
  uploadMedia: (productId, file) => {
    const fd = new FormData()
    fd.append('file', file)
    return request('POST', '/products/' + productId + '/media', fd, true)
  },
}
