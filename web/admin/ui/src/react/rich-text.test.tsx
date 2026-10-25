// 富文本迁移测试 / Rich Text Migration Tests
// 功能：验证浏览器挂载白名单、链接视频边界、选区恢复、纯文字粘贴和媒体插入契约
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { createRef } from 'react'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { afterEach, expect, it, vi } from 'vitest'
import { AdminTheme } from './theme'
import { RichTextEditor, type RichTextHandle } from './RichTextEditor'
import { sanitizeRichHTML, safeRichLink, videoEmbedURL } from './rich-html'
const image = `/media/blog/${'a'.repeat(64)}.webp`
const originalCommand = Object.getOwnPropertyDescriptor(document, 'execCommand')
afterEach(() => { if (originalCommand) Object.defineProperty(document, 'execCommand', originalCommand); else Reflect.deleteProperty(document, 'execCommand') })
function mount(html = '<p>Choose this text</p>', disabled = false) {
  const ref = createRef<RichTextHandle>(), upload = vi.fn()
  render(<AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><RichTextEditor ref={ref} initialHTML={html} disabled={disabled} upload={upload} /></ConfigProvider></AdminTheme>)
  return { ref, upload, editor: screen.getByRole('textbox', { name: '文章正文' }) }
}
function selectText(editor: HTMLElement) {
  const range = document.createRange(); range.selectNodeContents(editor.querySelector('p')!)
  window.getSelection()?.removeAllRanges(); window.getSelection()?.addRange(range); fireEvent.mouseUp(editor)
  return range.toString()
}
it('挂载白名单移除脚本/事件/外部图片/未知播放器，安全文本与本地图片保留', () => {
  const html = sanitizeRichHTML(`<script>bad()</script><svg onload="bad()"></svg><p onclick="bad()">Text &amp; &lt;safe&gt;<img src="${image}" onerror="bad()" alt="Local"></p><img src="https://outside.invalid/x.png"><iframe src="https://outside.invalid/"></iframe><a href="javascript:bad()">Label</a>`)
  expect(html).toContain('Text &amp; &lt;safe&gt;'); expect(html).toContain(`src="${image}"`); expect(html).toContain('Label')
  expect(html).not.toMatch(/script|svg|onerror|onclick|outside|javascript|<a/)
})
it('排版仅保留服务端可保存的颜色/字号/行高/字体/对齐，表格与旧 font 标签兼容', () => {
  const html = sanitizeRichHTML('<font color="#C2413B" size="4" face="serif">Title</font><p style="line-height:1.8;text-align:center;position:fixed;background-image:url(https://outside.invalid)">Body</p><table><tbody><tr><td>Cell</td></tr></tbody></table>')
  expect(html).toContain('font-size: 18px'); expect(html).toContain('font-family: serif'); expect(html).toContain('line-height: 1.8'); expect(html).toContain('text-align: center'); expect(html).toContain('<td>Cell</td>')
  expect(html).not.toMatch(/position|background-image|outside|<font/)
})
it('链接拒绝脚本、协议相对路径、反斜线和凭证，正文按 UTF-8 字节限制', () => {
  for (const value of ['javascript:bad()', '//outside.invalid', '/\\outside.invalid', 'https://name:password@outside.invalid', 'https://outside.invalid/<bad>']) expect(safeRichLink(value)).toBe(false)
  expect(safeRichLink('/products/running')).toBe(true); expect(safeRichLink('https://kartwo.com/blog')).toBe(true)
  expect(() => sanitizeRichHTML('中'.repeat(33334))).toThrow('正文过长')
})
it('视频链接只转换支持的 YouTube/Vimeo URL，HTML 嵌入只接受无参数规范播放器', () => {
  expect(videoEmbedURL('https://youtu.be/abcdefghijk?t=20')).toBe('https://www.youtube-nocookie.com/embed/abcdefghijk')
  expect(videoEmbedURL('https://www.youtube.com/watch?v=abcdefghijk')).toBe('https://www.youtube-nocookie.com/embed/abcdefghijk')
  expect(videoEmbedURL('https://vimeo.com/12345')).toBe('https://player.vimeo.com/video/12345')
  for (const value of ['http://youtu.be/abcdefghijk', 'https://youtu.be/short', 'https://www.youtube.com/watch?v=short', 'https://youtube.com.outside.invalid/watch?v=abcdefghijk', 'https://player.vimeo.com:8443/video/123']) expect(videoEmbedURL(value)).toBe('')
  const valid = '<iframe src="https://player.vimeo.com/video/123"></iframe>'
  expect(sanitizeRichHTML(valid)).toContain('referrerpolicy="no-referrer"')
  expect(sanitizeRichHTML(valid.replace('/123', '/123?autoplay=1'))).toBe('')
})
it('正文初始安全挂载，排版按钮恢复编辑区选中范围再执行原生命令', async () => {
  const command = vi.fn(() => true); Object.defineProperty(document, 'execCommand', { configurable: true, value: command })
  const { editor, ref } = mount('<p onclick="bad()">Choose this text</p>')
  const selected = selectText(editor)
  await userEvent.click(screen.getByRole('button', { name: '加粗' }))
  expect(command).toHaveBeenCalledWith('bold', false, undefined)
  expect(window.getSelection()?.toString()).toBe(selected)
  expect(ref.current?.getHTML()).toBe('<p>Choose this text</p>')
})
it('排版下拉在操作后清空选择，同一颜色可以反复应用到不同选区', async () => {
  const command = vi.fn((_type: string, _ui: boolean, _value?: string) => true); Object.defineProperty(document, 'execCommand', { configurable: true, value: command })
  const { editor } = mount(); selectText(editor)
  for (let count = 0; count < 2; count++) {
    await userEvent.click(screen.getByRole('combobox', { name: '文字颜色' }))
    await userEvent.click(screen.getByText('红色', { selector: '.ant-select-item-option-content' }))
  }
  expect(command.mock.calls.filter(([type]) => type === 'foreColor')).toHaveLength(2)
})
it('粘贴与拖入只接收纯文字，不能把外来 HTML 或文件节点挂进编辑器', () => {
  const { editor, ref } = mount('')
  fireEvent.paste(editor, { clipboardData: { getData: (type: string) => type === 'text/plain' ? '<img src=x onerror=bad()>' : '<script>bad()</script>' } })
  fireEvent.drop(editor, { dataTransfer: { getData: (type: string) => type === 'text/plain' ? ' Drop' : '<iframe src=x></iframe>' } })
  expect(editor.querySelector('img,script,iframe')).toBeNull(); expect(editor.textContent).toBe('<img src=x onerror=bad()> Drop')
  expect(ref.current?.getHTML()).toContain('&lt;img')
})
it('视频弹窗拒绝无效地址并保留输入，可在光标末尾插入受限播放器', async () => {
  const { editor } = mount('<p>Start</p>')
  await userEvent.click(screen.getByRole('button', { name: '视频' }))
  let dialog = screen.getByRole('dialog'); fireEvent.change(within(dialog).getByLabelText('视频地址'), { target: { value: 'https://outside.invalid/video' } })
  await userEvent.click(within(dialog).getByRole('button', { name: /插\s*入/ }))
  expect(within(dialog).getByText('请使用有效的 YouTube 或 Vimeo HTTPS 视频链接')).toBeTruthy()
  fireEvent.change(within(dialog).getByLabelText('视频地址'), { target: { value: 'https://vimeo.com/12345' } })
  await userEvent.click(within(dialog).getByRole('button', { name: /插\s*入/ }))
  expect(editor.querySelector('iframe')?.getAttribute('src')).toBe('https://player.vimeo.com/video/12345')
  expect(editor.textContent).toContain('Start')
})
it('链接必须先选中文字，弹窗恢复保存的选区', async () => {
  const command = vi.fn(() => true); Object.defineProperty(document, 'execCommand', { configurable: true, value: command })
  const { editor } = mount()
  await userEvent.click(screen.getByRole('button', { name: '链接' })); expect(screen.queryByRole('dialog')).toBeNull()
  selectText(editor); await userEvent.click(screen.getByRole('button', { name: '链接' }))
  const dialog = screen.getByRole('dialog'); fireEvent.change(within(dialog).getByLabelText('链接地址'), { target: { value: '/blog/story' } })
  await userEvent.click(within(dialog).getByRole('button', { name: /插\s*入/ }))
  expect(command).toHaveBeenLastCalledWith('createLink', false, '/blog/story'); expect(window.getSelection()?.toString()).toBe('Choose this text')
})
it('上传回调图片经过路径过滤插入', async () => {
  const { ref, editor } = mount('<p>Text before image</p>')
  await act(async () => ref.current?.insertImage(image))
  expect(ref.current?.getHTML()).toContain(`src="${image}"`); expect(editor.textContent).toContain('Text before image')
})
it('只读模式的工具栏、粘贴和媒体插入都不可写', async () => {
  const { ref, editor, upload } = mount('<p>Read me</p>', true)
  expect(editor.getAttribute('contenteditable')).toBe('false')
  expect((screen.getByRole('button', { name: '图片' }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.paste(editor, { clipboardData: { getData: () => 'Forbidden' } }); expect(editor.textContent).toBe('Read me'); expect(upload).not.toHaveBeenCalled()
  await act(async () => ref.current?.insertImage(image))
  expect(editor.querySelector('img')).toBeNull()
})
