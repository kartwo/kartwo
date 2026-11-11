// 博客富文本编辑器 / Blog Rich Text Editor
// 功能：保留多行工具栏与原生编辑撤销栈，安全插入图片/表格/链接/视频，阻止 HTML 粘贴与拖入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { forwardRef, useEffect, useImperativeHandle, useRef, useState, type ClipboardEvent, type DragEvent } from 'react'
import { Alert, App, Button, Input, Modal, Select } from 'antd'
import { sanitizeRichHTML, safeRichLink, videoEmbedURL } from './rich-html'
import { AlignLeftOutlined, BgColorsOutlined, BoldOutlined, CheckSquareOutlined, ColumnHeightOutlined, CommentOutlined, FontColorsOutlined, FontSizeOutlined, ItalicOutlined, LinkOutlined, MenuFoldOutlined, MenuUnfoldOutlined, OrderedListOutlined, PictureOutlined, RedoOutlined, SmileOutlined, StrikethroughOutlined, TableOutlined, UnderlineOutlined, UndoOutlined, UnorderedListOutlined, VideoCameraOutlined } from '@ant-design/icons'
const formatIcons: Record<string, typeof BoldOutlined> = { 引用: CommentOutlined, 加粗: BoldOutlined, 斜体: ItalicOutlined, 下划线: UnderlineOutlined, 删除线: StrikethroughOutlined, 项目列表: UnorderedListOutlined, 编号列表: OrderedListOutlined, 待办列表: CheckSquareOutlined, 增加缩进: MenuUnfoldOutlined, 减少缩进: MenuFoldOutlined, 撤销: UndoOutlined, 重做: RedoOutlined }
const selectIcons: Record<string, typeof BoldOutlined> = { 文字颜色: FontColorsOutlined, 背景颜色: BgColorsOutlined, 字号: FontSizeOutlined, 字体: FontSizeOutlined, 行高: ColumnHeightOutlined, 对齐方式: AlignLeftOutlined, 插入表情: SmileOutlined, 插入表格: TableOutlined }
export interface RichTextHandle { getHTML: () => string; insertImage: (url: string) => void }
interface Props { initialHTML: string; disabled: boolean; upload: (file: File) => void }
// These are merchant content colors, not application UI colors; the server keeps only safe color literals.
const textColors = [['#25344f', '深蓝'], ['#c2413b', '红色'], ['#1f7669', '绿色'], ['#965c1c', '棕色']]
const backgrounds = [['#fff1b8', '浅黄'], ['#dceafe', '浅蓝'], ['#e4f4e7', '浅绿']]
export const RichTextEditor = forwardRef<RichTextHandle, Props>(function RichTextEditor({ initialHTML, disabled, upload }, ref) {
  const editor = useRef<HTMLDivElement>(null), imageInput = useRef<HTMLInputElement>(null), savedRange = useRef<Range | null>(null)
  const { message } = App.useApp()
  const [dialog, setDialog] = useState<'link' | 'video' | null>(null), [url, setURL] = useState(''), [error, setError] = useState('')
  useEffect(() => { if (editor.current) editor.current.innerHTML = sanitizeRichHTML(initialHTML); savedRange.current = null }, [initialHTML])
  function remember() {
    const selection = window.getSelection()
    if (!selection?.rangeCount || !editor.current) return
    const range = selection.getRangeAt(0)
    if (editor.current.contains(range.startContainer) && editor.current.contains(range.endContainer)) savedRange.current = range.cloneRange()
  }
  function restore(): Range | null {
    if (!editor.current) return null
    editor.current.focus()
    const range = savedRange.current && editor.current.contains(savedRange.current.startContainer) && editor.current.contains(savedRange.current.endContainer)
      ? savedRange.current : document.createRange()
    if (range !== savedRange.current) { range.selectNodeContents(editor.current); range.collapse(false) }
    const selection = window.getSelection(); selection?.removeAllRanges(); selection?.addRange(range)
    return range
  }
  function format(command: string, value?: string): boolean {
    if (disabled || !restore()) return false
    try {
      const done = typeof document.execCommand === 'function' && document.execCommand(command, false, value)
      remember(); return done
    } catch { return false }
  }
  function run(command: string, value?: string) {
    if (!format(command, value) && !disabled) void message.error('浏览器未完成该排版操作，请重新选中文字后重试')
  }
  function insert(html: string) {
    if (disabled || !editor.current) return
    const clean = sanitizeRichHTML(html)
    // Native insertHTML preserves undo. The Range fallback also supports browsers that decline it.
    const range = restore()
    if (!range) return
    if (format('insertHTML', clean)) return
    const template = document.createElement('template'); template.innerHTML = clean
    const last = template.content.lastChild
    range.deleteContents(); range.insertNode(template.content)
    if (last) { range.setStartAfter(last); range.collapse(true); window.getSelection()?.removeAllRanges(); window.getSelection()?.addRange(range) }
    remember()
  }
  useImperativeHandle(ref, () => ({
    getHTML: () => { if (!editor.current) throw new Error('正文编辑器尚未就绪'); return sanitizeRichHTML(editor.current.innerHTML) },
    insertImage: source => { const img = document.createElement('img'); img.src = source; img.alt = '图片说明'; insert(`<p>${img.outerHTML}</p><p><br></p>`) },
  }))
  function plainText(event: ClipboardEvent<HTMLDivElement> | DragEvent<HTMLDivElement>) {
    event.preventDefault()
    if (disabled) return
    const text = 'clipboardData' in event ? event.clipboardData.getData('text/plain') : event.dataTransfer.getData('text/plain')
    if (!format('insertText', text)) {
      const range = restore(); if (!range) return
      const node = document.createTextNode(text); range.deleteContents(); range.insertNode(node); range.setStartAfter(node); range.collapse(true); remember()
    }
  }
  function block(property: 'lineHeight' | 'textAlign', value: string) {
    if (disabled || !restore()) return
    let anchor = window.getSelection()?.anchorNode
    if (anchor?.nodeType === Node.TEXT_NODE) anchor = anchor.parentElement
    let element = anchor instanceof Element ? anchor.closest('p,h2,h3,li,blockquote,div') : null
    if (!element || element === editor.current) {
      format('formatBlock', 'p'); anchor = window.getSelection()?.anchorNode
      if (anchor?.nodeType === Node.TEXT_NODE) anchor = anchor.parentElement
      element = anchor instanceof Element ? anchor.closest('p,h2,h3,li,blockquote,div') : null
    }
    if (element instanceof HTMLElement && element !== editor.current && editor.current?.contains(element)) element.style[property] = value
    remember()
  }
  function open(kind: 'link' | 'video') {
    if (disabled) return
    if (kind === 'link' && (!savedRange.current || savedRange.current.collapsed || !editor.current?.contains(savedRange.current.commonAncestorContainer))) {
      void message.error('请先在正文中选中要加链接的文字'); return
    }
    setURL(''); setError(''); setDialog(kind)
  }
  function confirm() {
    if (disabled) return
    if (dialog === 'link') {
      if (!safeRichLink(url)) { setError('链接须为有效的 HTTPS 地址或以 / 开头的站内路径'); return }
      if (!format('createLink', url)) { setError('链接未插入，请重新选中文字后重试'); return }
    } else {
      const source = videoEmbedURL(url)
      if (!source) { setError('请使用有效的 YouTube 或 Vimeo HTTPS 视频链接'); return }
      insert(`<iframe src="${source}" title="Video" loading="lazy" allowfullscreen></iframe><p><br></p>`)
    }
    setDialog(null)
  }
  const button = (label: string, text: string, command: string, value?: string) => { const Icon = formatIcons[label]; return <Button key={label} icon={Icon ? <Icon aria-hidden /> : undefined} aria-label={label} title={label} disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => run(command, value)}>{Icon ? undefined : text}</Button> }
  const select = (label: string, values: readonly (readonly string[])[], change: (value: string) => void) => { const Icon = selectIcons[label]; return <Select prefix={Icon && <Icon aria-hidden />} aria-label={label} placeholder={label} value={null} disabled={disabled} onMouseDown={remember} onChange={change} options={values.map(([value, text]) => ({ value: value!, label: text! }))} /> }
  return <div className="blog-rich-text">
    <div className="blog-rich-toolbar" role="toolbar" aria-label="正文排版">
      <div className="blog-toolbar-row">
        {button('引用', '❝', 'formatBlock', 'blockquote')}{button('二级标题', 'H2', 'formatBlock', 'h2')}{button('三级标题', 'H3', 'formatBlock', 'h3')}
        {button('加粗', 'B', 'bold')}{button('斜体', 'I', 'italic')}{button('下划线', 'U', 'underline')}{button('删除线', 'S', 'strikeThrough')}
        {select('文字颜色', textColors, value => run('foreColor', value))}{select('背景颜色', backgrounds, value => run('hiliteColor', value))}
        {select('字号', [['2', '小'], ['3', '标准'], ['4', '大'], ['5', '特大']], value => run('fontSize', value))}
      </div>
      <div className="blog-toolbar-row">
        {select('字体', [['sans-serif', '无衬线'], ['serif', '衬线'], ['monospace', '等宽']], value => run('fontName', value))}
        {select('行高', [['1.2', '紧凑'], ['1.5', '标准'], ['1.8', '宽松'], ['2', '双倍']], value => block('lineHeight', value))}
        {button('项目列表', '项目列表', 'insertUnorderedList')}{button('编号列表', '编号列表', 'insertOrderedList')}{button('待办列表', '☑', 'insertHTML', '<ul><li>☐ 待办事项</li></ul>')}
        {select('对齐方式', [['left', '左对齐'], ['center', '居中'], ['right', '右对齐'], ['justify', '两端对齐']], value => block('textAlign', value))}
        {button('增加缩进', '⇥', 'indent')}{button('减少缩进', '⇤', 'outdent')}
        {select('插入表情', [['😀', '😀'], ['❤️', '❤️'], ['👍', '👍'], ['✨', '✨']], value => run('insertText', value))}
        <Button icon={<PictureOutlined aria-hidden />} aria-label="图片" disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => { remember(); imageInput.current?.click() }}>图片</Button>
        <Button icon={<VideoCameraOutlined aria-hidden />} aria-label="视频" disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => open('video')}>视频</Button>
      </div>
      <div className="blog-toolbar-row">
        <Button icon={<LinkOutlined aria-hidden />} aria-label="链接" disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => open('link')}>链接</Button>
        {select('插入表格', [['2', '2 × 2'], ['3', '3 × 3'], ['4', '4 × 4']], value => { const n = Number(value); insert(`<table><tbody>${Array.from({ length: n }, () => `<tr>${'<td><br></td>'.repeat(n)}</tr>`).join('')}</tbody></table><p><br></p>`) })}
        {button('撤销', '↶', 'undo')}{button('重做', '↷', 'redo')}
      </div>
    </div>
    <div ref={editor} className="blog-rich-editor" contentEditable={!disabled} suppressContentEditableWarning role="textbox" aria-label="文章正文" aria-multiline="true" aria-readonly={disabled} tabIndex={0} data-placeholder="请输入正文…" onInput={remember} onMouseUp={remember} onKeyUp={remember} onPaste={plainText} onDrop={plainText} />
    <input ref={imageInput} className="blog-image-input" type="file" accept="image/jpeg,image/png,image/webp" disabled={disabled} aria-label="上传正文图片" onChange={event => { const file = event.currentTarget.files?.[0]; event.currentTarget.value = ''; if (file && !disabled) upload(file) }} />
    <Modal open={dialog !== null} title={dialog === 'link' ? '插入链接' : '插入视频'} okText="插入" cancelText="取消" onCancel={() => setDialog(null)} onOk={confirm} okButtonProps={{ disabled }}>
      {error && <Alert type="error" showIcon title={error} />}<Input autoFocus aria-label={dialog === 'link' ? '链接地址' : '视频地址'} value={url} onChange={event => setURL(event.target.value)} onPressEnter={confirm} />
    </Modal>
  </div>
})
