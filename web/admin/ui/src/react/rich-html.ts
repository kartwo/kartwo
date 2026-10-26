// 富文本浏览器防护 / Rich Text Browser Protection
// 功能：挂载前再过滤富文本，限制链接、本地图片、视频及排版；服务端仍是存储与店面安全依据
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { blogImagePath } from './blog-api'
export function safeRichLink(raw: string): boolean {
  if (!raw || /[\s<>"'\\]/.test(raw)) return false
  if (/^\/(?!\/)/.test(raw)) return true
  try { const url = new URL(raw); return url.protocol === 'https:' && !!url.hostname && !url.username && !url.password } catch { return false }
}
export function videoEmbedURL(raw: string): string {
  try {
    const url = new URL(raw)
    if (url.protocol !== 'https:' || url.username || url.password || url.port) return ''
    let id: string | null | undefined = ''
    if (['youtube.com', 'www.youtube.com', 'm.youtube.com'].includes(url.hostname)) id = url.pathname === '/watch' ? url.searchParams.get('v') : url.pathname.match(/^\/(?:shorts|embed)\/([\w-]{11})$/)?.[1]
    if (url.hostname === 'youtu.be') id = url.pathname.match(/^\/([\w-]{11})$/)?.[1]
    if (url.hostname === 'www.youtube-nocookie.com') id = url.pathname.match(/^\/embed\/([\w-]{11})$/)?.[1]
    if (id && /^[\w-]{11}$/.test(id)) return `https://www.youtube-nocookie.com/embed/${id}`
    const vimeo = url.hostname === 'vimeo.com' ? url.pathname.match(/^\/([0-9]{1,20})$/)?.[1]
      : url.hostname === 'player.vimeo.com' ? url.pathname.match(/^\/video\/([0-9]{1,20})$/)?.[1] : ''
    if (vimeo) return `https://player.vimeo.com/video/${vimeo}`
  } catch { /* Invalid or unsupported links do not become iframe sources. */ }
  return ''
}
const tags: Record<string, string> = { p: 'p', div: 'p', h1: 'h2', h2: 'h2', h3: 'h3', ul: 'ul', ol: 'ol', li: 'li', blockquote: 'blockquote',
  strong: 'strong', b: 'strong', em: 'em', i: 'em', u: 'u', s: 's', strike: 's', del: 's', span: 'span', font: 'span',
  table: 'table', thead: 'thead', tbody: 'tbody', tr: 'tr', th: 'th', td: 'td', br: 'br' }
const blocked = new Set(['script', 'style', 'object', 'embed', 'svg', 'math', 'template', 'form', 'textarea', 'button', 'input'])
function color(raw: string) {
  if (/^#[a-f0-9]{6}$/i.test(raw)) return raw.toLowerCase()
  const values = raw.match(/^rgb\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*\)$/)
  if (!values) return ''
  const channels = values.slice(1).map(Number)
  return channels.every(n => n <= 255) ? '#' + channels.map(n => n.toString(16).padStart(2, '0')).join('') : ''
}
function copyStyle(source: HTMLElement, target: HTMLElement) {
  for (const property of ['color', 'background-color', 'font-family', 'font-size', 'line-height', 'text-align']) {
    let value = source.style.getPropertyValue(property).trim().toLowerCase()
    if (source.tagName === 'FONT') {
      if (property === 'color') value = source.getAttribute('color') || value
      if (property === 'font-family') value = source.getAttribute('face') || value
      if (property === 'font-size') value = ({ '2': '13px', '3': '16px', '4': '18px', '5': '24px', '6': '32px' } as Record<string, string>)[source.getAttribute('size') ?? ''] || value
    }
    if (property === 'color' || property === 'background-color') value = color(value)
    else if (property === 'font-family' && !['sans-serif', 'serif', 'monospace'].includes(value)) value = ''
    else if (property === 'font-size' && !/^(?:1[2-9]|2\d|3[0-2])px$/.test(value)) value = ''
    else if (property === 'line-height' && !['1.2', '1.5', '1.8', '2'].includes(value)) value = ''
    else if (property === 'text-align' && !['left', 'center', 'right', 'justify'].includes(value)) value = ''
    if (value) target.style.setProperty(property, value)
  }
}
export function sanitizeRichHTML(raw: string): string {
  if (new TextEncoder().encode(raw).length > 100000) throw new Error('文章正文过长，请减少内容后重试')
  // template content remains inert: no untrusted nodes are ever mounted in the live editor.
  const source = document.createElement('template'), target = document.createElement('div')
  source.innerHTML = raw
  function append(node: Node, parent: Node) {
    if (node.nodeType === Node.TEXT_NODE) { parent.appendChild(document.createTextNode(node.textContent ?? '')); return }
    if (!(node instanceof Element)) return
    const tag = node.tagName.toLowerCase()
    if (blocked.has(tag)) return
    let clean: HTMLElement | null = null
    if (tag === 'img') {
      const src = node.getAttribute('src') ?? ''
      if (!src.startsWith('/media/') || !blogImagePath.test(src.slice('/media/'.length))) return
      clean = document.createElement('img'); clean.setAttribute('src', src); clean.setAttribute('alt', node.getAttribute('alt') ?? ''); clean.setAttribute('loading', 'lazy')
    } else if (tag === 'iframe') {
      const src = node.getAttribute('src') ?? ''
      // Embed nodes accept only canonical player URLs, without query, fragment or alternate host.
      if (!src || videoEmbedURL(src) !== src) return
      clean = document.createElement('iframe'); clean.setAttribute('src', src); clean.setAttribute('title', 'Video'); clean.setAttribute('loading', 'lazy'); clean.setAttribute('referrerpolicy', src.startsWith('https://www.youtube-nocookie.com/embed/') ? 'strict-origin' : 'no-referrer'); clean.setAttribute('allowfullscreen', '')
    } else if (tag === 'a' && safeRichLink(node.getAttribute('href') ?? '')) {
      clean = document.createElement('a'); clean.setAttribute('href', node.getAttribute('href')!); clean.setAttribute('rel', 'noopener noreferrer')
    } else if (Object.hasOwn(tags, tag)) {
      clean = document.createElement(tags[tag]!); if (node instanceof HTMLElement) copyStyle(node, clean)
    }
    const container = clean ?? parent
    if (tag !== 'img' && tag !== 'iframe') Array.from(node.childNodes).forEach(child => append(child, container))
    if (clean) parent.appendChild(clean)
  }
  Array.from(source.content.childNodes).forEach(node => append(node, target))
  return target.innerHTML
}
