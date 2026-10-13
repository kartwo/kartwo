<!-- 博客文章管理 / Blog Post Management
功能：草稿、预览、发布、封面和正文图片上传
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-10-13 10:00:00 -->
<script setup>
import { ref, computed, inject, onMounted, nextTick } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'
const toast = useToast()
const onUnauthorized = inject('onUnauthorized')
const demo = inject('demoSession', ref(false))
const role = inject('accountRole', ref(''))
const permissions = inject('accountPermissions', ref([]))
const canManage = computed(() => !demo.value && (role.value === 'owner' || permissions.value.includes('blog.manage')))
const posts = ref([])
const form = ref(empty())
const editor = ref(null)
const bodyImageInput = ref(null)
const uploading = ref(false)
let savedRange = null
function empty() { return { public_id:'', title:'', slug:'', excerpt:'', body_markdown:'', body_html:'', cover_path:'', seo_description:'', status:'draft' } }
async function load() { try { posts.value = (await api.listBlogPosts()).posts || [] } catch(e) { if(e instanceof APIError && e.status===401) onUnauthorized(); else toast.error(e.message) } }
function reset() { form.value = empty(); savedRange = null; if(editor.value) editor.value.innerHTML = '' }
async function edit(post) { try { const loaded = await api.getBlogPost(post.public_id); const html = loaded.body_html || (loaded.body_markdown ? (await api.previewBlog(loaded.body_markdown)).html : ''); form.value = loaded; savedRange = null; await nextTick(); if(editor.value) editor.value.innerHTML = html || '' } catch(e) { toast.error(e.message) } }
async function save() { try { form.value.body_html = editor.value?.innerHTML || ''; form.value.body_markdown = ''; if(form.value.public_id) await api.updateBlogPost(form.value.public_id,form.value); else { const r=await api.createBlogPost(form.value); form.value.public_id=r.public_id } await load();toast.success('文章已保存') } catch(e) { toast.error(e.message) } }
async function remove(post) { if(!await confirm({title:'删除文章',message:`确定删除「${post.title}」？`,confirmText:'删除',danger:true})) return; try { await api.deleteBlogPost(post.public_id); if(form.value.public_id===post.public_id) reset(); await load(); toast.success('文章已删除') } catch(e) { toast.error(e.message) } }
function rememberSelection() { const selection = window.getSelection(); if(selection?.rangeCount && editor.value?.contains(selection.anchorNode)) savedRange = selection.getRangeAt(0).cloneRange() }
function restoreSelection() { editor.value?.focus(); if(savedRange) { const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(savedRange) } }
function format(command, value) { restoreSelection(); const done = document.execCommand(command, false, value); rememberSelection(); return done }
function choice(event, command) { if(event.target.value) format(command, event.target.value); event.target.value = '' }
function blockStyle(event, property) {
  const value = event.target.value
  event.target.value = ''
  if(!value) return
  restoreSelection()
  let node = window.getSelection()?.anchorNode
  if(node?.nodeType === Node.TEXT_NODE) node = node.parentElement
  let block = node?.closest?.('p,h2,h3,li,blockquote,div')
  if(!block || block === editor.value || !editor.value?.contains(block)) {
    document.execCommand('formatBlock', false, 'p')
    node = window.getSelection()?.anchorNode
    if(node?.nodeType === Node.TEXT_NODE) node = node.parentElement
    block = node?.closest?.('p,h2,h3,li,blockquote,div')
  }
  if(block && block !== editor.value && editor.value?.contains(block)) block.style[property] = value
  rememberSelection()
}
function insertLink() {
  const range = savedRange
  if(!range || range.collapsed || !editor.value?.contains(range.commonAncestorContainer)) { toast.error('请先在正文中选中要加链接的文字'); return }
  const url = window.prompt('请输入 HTTPS 链接或站内路径（以 / 开头）')
  if(url === null) return
  let external = false
  try { const parsed = new URL(url); external = parsed.protocol === 'https:' && !!parsed.hostname && !parsed.username && !parsed.password } catch (_) {}
  if(!external && !/^\/(?!\/)[^\s]*$/.test(url)) { toast.error('链接须以 https:// 或 / 开头'); return }
  format('createLink', url)
}
function pastePlainText(event) { event.preventDefault(); document.execCommand('insertText', false, event.clipboardData?.getData('text/plain') || '') }
function insertTable(event) {
  const size = Number(event.target.value)
  event.target.value = ''
  if(!size) return
  const rows = Array.from({ length: size }, () => '<tr>' + '<td><br></td>'.repeat(size) + '</tr>').join('')
  format('insertHTML', `<table><tbody>${rows}</tbody></table><p><br></p>`)
}
function videoEmbedURL(raw) {
  try {
    const url = new URL(raw)
    if(url.protocol !== 'https:' || url.username || url.password) return ''
    let id = ''
    if(['youtube.com','www.youtube.com','m.youtube.com'].includes(url.hostname)) id = url.pathname === '/watch' ? url.searchParams.get('v') : url.pathname.match(/^\/(?:shorts|embed)\/([\w-]{11})$/)?.[1]
    if(url.hostname === 'youtu.be') id = url.pathname.match(/^\/([\w-]{11})$/)?.[1]
    if(url.hostname === 'www.youtube-nocookie.com') id = url.pathname.match(/^\/embed\/([\w-]{11})$/)?.[1]
    if(id && /^[\w-]{11}$/.test(id)) return `https://www.youtube-nocookie.com/embed/${id}`
    if(url.hostname === 'vimeo.com') id = url.pathname.match(/^\/([0-9]{1,20})$/)?.[1]
    if(url.hostname === 'player.vimeo.com') id = url.pathname.match(/^\/video\/([0-9]{1,20})$/)?.[1]
    if(id) return `https://player.vimeo.com/video/${id}`
  } catch (_) {}
  return ''
}
function insertVideo() {
  const raw = window.prompt('粘贴 YouTube 或 Vimeo 视频链接')
  if(raw === null) return
  const src = videoEmbedURL(raw)
  if(!src) { toast.error('请使用有效的 YouTube 或 Vimeo HTTPS 视频链接'); return }
  format('insertHTML', `<p><iframe src="${src}" title="Video" loading="lazy" allowfullscreen></iframe></p><p><br></p>`)
}
async function upload(event, cover) { const file=event.target.files?.[0]; if(!file)return; uploading.value=true; try { const image=await api.uploadBlogImage(file); if(cover) form.value.cover_path=image.path; else if(!format('insertHTML', `<p><img src="${image.url}" alt="图片说明"></p>`)) editor.value?.insertAdjacentHTML('beforeend', `<p><img src="${image.url}" alt="图片说明"></p>`); toast.success('图片已上传') } catch(e) { toast.error(e.message) } finally { uploading.value=false; event.target.value='' } }
onMounted(load)
</script>
<template>
  <div v-if="canManage" class="page-toolbar page-toolbar--end"><button class="primary" @click="reset">新建文章</button></div>
  <p class="page-description">先保存草稿，确认内容后再发布。文章链接发布后保持不变。</p>
  <p v-if="demo" class="muted">公开演示中不显示真实文章草稿。</p>
  <section class="blog-layout">
    <div class="panel blog-list">
      <h3>文章列表</h3>
      <p v-if="!posts.length" class="muted empty-list">还没有文章。</p>
      <div v-for="post in posts" :key="post.public_id" class="blog-row">
        <button class="blog-title" @click="edit(post)">{{ post.title }}</button>
        <span class="chip" :class="{ draft: post.status !== 'active' }">{{ post.status === 'active' ? '已发布' : '草稿' }}</span>
        <button v-if="canManage" class="danger" @click="remove(post)">删除</button>
      </div>
    </div>
    <form class="panel blog-editor" :inert="!canManage" @submit.prevent="save">
      <h3>{{ form.public_id ? '编辑文章' : '新建文章' }}</h3>
      <label>标题</label><input v-model="form.title" maxlength="160" required />
      <label>文章链接</label><input v-model="form.slug" pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="100" :disabled="!!form.public_id" placeholder="my-story" required />
      <label>摘要</label><textarea v-model="form.excerpt" maxlength="300" rows="3" />
      <label>SEO 描述</label><textarea v-model="form.seo_description" maxlength="300" rows="2" />
      <label>封面图</label><input type="file" accept="image/jpeg,image/png,image/webp" :disabled="uploading" @change="upload($event,true)" />
      <img v-if="form.cover_path" class="cover-preview" :src="'/media/'+form.cover_path" alt="封面预览" />
      <label>正文</label>
      <div class="rich-toolbar" role="toolbar" aria-label="正文排版">
        <div class="toolbar-row">
          <div class="tool-group">
            <button type="button" title="引用" aria-label="引用" @mousedown.prevent @click="format('formatBlock','blockquote')">❝</button>
            <button type="button" @mousedown.prevent @click="format('formatBlock','h2')">H2</button>
            <button type="button" @mousedown.prevent @click="format('formatBlock','h3')">H3</button>
          </div>
          <div class="tool-group">
            <button type="button" title="加粗" aria-label="加粗" @mousedown.prevent @click="format('bold')"><strong>B</strong></button>
            <button type="button" title="斜体" aria-label="斜体" @mousedown.prevent @click="format('italic')"><em>I</em></button>
            <button type="button" title="下划线" aria-label="下划线" @mousedown.prevent @click="format('underline')"><u>U</u></button>
            <button type="button" title="删除线" aria-label="删除线" @mousedown.prevent @click="format('strikeThrough')"><s>S</s></button>
          </div>
          <div class="tool-group">
            <select aria-label="文字颜色" title="文字颜色" @change="choice($event,'foreColor')"><option value="">文字颜色</option><option value="#25344f">深蓝</option><option value="#c2413b">红色</option><option value="#1f7669">绿色</option><option value="#965c1c">棕色</option></select>
            <select aria-label="背景颜色" title="背景颜色" @change="choice($event,'hiliteColor')"><option value="">背景颜色</option><option value="#fff1b8">浅黄</option><option value="#dceafe">浅蓝</option><option value="#e4f4e7">浅绿</option></select>
            <select aria-label="字号" title="字号" @change="choice($event,'fontSize')"><option value="">默认字号</option><option value="2">小</option><option value="3">标准</option><option value="4">大</option><option value="5">特大</option></select>
          </div>
        </div>
        <div class="toolbar-row">
          <div class="tool-group">
            <select aria-label="字体" title="字体" @change="choice($event,'fontName')"><option value="">默认字体</option><option value="sans-serif">无衬线</option><option value="serif">衬线</option><option value="monospace">等宽</option></select>
            <select aria-label="行高" title="行高" @change="blockStyle($event,'lineHeight')"><option value="">默认行高</option><option value="1.2">紧凑</option><option value="1.5">标准</option><option value="1.8">宽松</option><option value="2">双倍</option></select>
          </div>
          <div class="tool-group">
            <button type="button" title="项目列表" aria-label="项目列表" @mousedown.prevent @click="format('insertUnorderedList')">☷</button>
            <button type="button" title="编号列表" aria-label="编号列表" @mousedown.prevent @click="format('insertOrderedList')">1☷</button>
            <button type="button" title="待办列表" aria-label="待办列表" @mousedown.prevent @click="format('insertHTML','<ul><li>☐ 待办事项</li></ul>')">☑</button>
          </div>
          <div class="tool-group">
            <select aria-label="对齐方式" title="对齐方式" @change="blockStyle($event,'textAlign')"><option value="">对齐方式</option><option value="left">左对齐</option><option value="center">居中</option><option value="right">右对齐</option><option value="justify">两端对齐</option></select>
            <button type="button" title="增加缩进" aria-label="增加缩进" @mousedown.prevent @click="format('indent')">⇥</button>
            <button type="button" title="减少缩进" aria-label="减少缩进" @mousedown.prevent @click="format('outdent')">⇤</button>
            <select aria-label="插入表情" title="插入表情" @change="choice($event,'insertText')"><option value="">表情</option><option value="😀">😀</option><option value="❤️">❤️</option><option value="👍">👍</option><option value="✨">✨</option></select>
          </div>
          <div class="tool-group tool-group--media">
            <button type="button" :disabled="uploading" @mousedown.prevent @click="bodyImageInput?.click()">▧ 图片</button>
            <button type="button" @mousedown.prevent @click="insertVideo">▣ 视频</button>
          </div>
        </div>
        <div class="toolbar-row">
          <div class="tool-group">
            <button type="button" title="插入链接" aria-label="插入链接" @mousedown.prevent @click="insertLink">🔗 链接</button>
            <select aria-label="插入表格" title="插入表格" @change="insertTable"><option value="">▦ 表格</option><option value="2">2 × 2</option><option value="3">3 × 3</option><option value="4">4 × 4</option></select>
          </div>
          <div class="tool-group">
            <button type="button" title="撤销" aria-label="撤销" @mousedown.prevent @click="format('undo')">↶</button>
            <button type="button" title="重做" aria-label="重做" @mousedown.prevent @click="format('redo')">↷</button>
          </div>
        </div>
      </div>
      <div ref="editor" class="rich-editor" contenteditable="true" role="textbox" aria-label="文章正文" aria-multiline="true" data-placeholder="请输入正文…" @paste="pastePlainText" @mouseup="rememberSelection" @keyup="rememberSelection" @input="rememberSelection"></div>
      <p class="muted editor-hint">选中文字后使用工具栏排版；视频支持 YouTube 和 Vimeo 链接。</p>
      <input ref="bodyImageInput" class="hidden-image-input" type="file" accept="image/jpeg,image/png,image/webp" :disabled="uploading" aria-label="上传正文图片" @change="upload($event,false)" />
      <p v-if="uploading" class="muted">正在上传并处理图片…</p>
      <label>状态</label><select v-model="form.status"><option value="draft">草稿</option><option value="active">发布</option></select>
      <div class="blog-actions"><button v-if="canManage" class="primary">保存文章</button></div>
    </form>
  </section>
</template>
<style scoped>
.blog-layout{display:grid;grid-template-columns:minmax(0,1fr);align-items:start;gap:var(--sp-4)}
.blog-list h3,.blog-editor h3{margin:0 0 var(--sp-4);font-size:var(--fs-lg)}
.empty-list{padding:var(--sp-6) 0}
.blog-row{display:flex;align-items:center;gap:var(--sp-2);padding:var(--sp-3) 0;border-bottom:1px solid var(--border)}
.blog-row:last-child{border-bottom:0}
.blog-title{min-width:0;flex:1;border:0;box-shadow:none;background:transparent;text-align:left;font-weight:600;overflow-wrap:anywhere}
.blog-title:hover{color:var(--accent);background:var(--surface-2)}
.blog-actions{display:flex;gap:var(--sp-3);margin-top:var(--sp-6)}
.cover-preview{display:block;max-width:240px;max-height:160px;object-fit:cover;margin:var(--sp-2) 0;border-radius:var(--radius-md)}
.editor-hint{font-size:var(--fs-sm)}
.rich-toolbar{border:1px solid var(--border);border-bottom:0;border-radius:var(--radius-sm) var(--radius-sm) 0 0;background:var(--surface)}
.toolbar-row{display:flex;flex-wrap:wrap;align-items:center;padding:var(--sp-2);border-bottom:1px solid var(--border)}
.toolbar-row:last-child{border-bottom:0}
.tool-group{display:flex;flex-wrap:wrap;align-items:center;gap:var(--sp-1);padding:0 var(--sp-3);border-right:1px solid var(--border)}
.tool-group:first-child{padding-left:0}.tool-group:last-child{border-right:0}.tool-group--media{margin-left:auto}
.rich-toolbar button,.rich-toolbar select{min-height:38px;width:auto;margin:0;padding:var(--sp-2);border:0;box-shadow:none;background:transparent;color:var(--text);font-size:var(--fs-sm);white-space:nowrap}
.rich-toolbar button:hover,.rich-toolbar select:hover{background:var(--surface-2)}
.rich-editor{min-height:420px;padding:var(--sp-4);border:1px solid var(--border);border-radius:0 0 var(--radius-sm) var(--radius-sm);background:var(--surface);line-height:1.7;overflow-wrap:anywhere;outline:none}
.rich-editor:focus{border-color:var(--accent)}
.rich-editor:empty:before{content:attr(data-placeholder);color:var(--muted);pointer-events:none}
.rich-editor :deep(img){max-width:100%;height:auto}
.rich-editor :deep(iframe){display:block;width:min(100%,640px);aspect-ratio:16/9;border:0}
.rich-editor :deep(table){border-collapse:collapse;width:100%;margin:var(--sp-3) 0}
.rich-editor :deep(td),.rich-editor :deep(th){border:1px solid var(--border);min-width:4rem;padding:var(--sp-2)}
.rich-editor :deep(blockquote){border-left:3px solid var(--accent);margin:var(--sp-3) 0;padding-left:var(--sp-3);color:var(--muted)}
.hidden-image-input{display:none}
[inert]{opacity:.7}
@media(max-width:800px){.tool-group{padding:var(--sp-1) var(--sp-2)}.tool-group--media{margin-left:0}}
</style>
