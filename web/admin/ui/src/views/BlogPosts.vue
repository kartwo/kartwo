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
const uploading = ref(false)
function empty() { return { public_id:'', title:'', slug:'', excerpt:'', body_markdown:'', body_html:'', cover_path:'', seo_description:'', status:'draft' } }
async function load() { try { posts.value = (await api.listBlogPosts()).posts || [] } catch(e) { if(e instanceof APIError && e.status===401) onUnauthorized(); else toast.error(e.message) } }
function reset() { form.value = empty(); if(editor.value) editor.value.innerHTML = '' }
async function edit(post) { try { const loaded = await api.getBlogPost(post.public_id); const html = loaded.body_html || (loaded.body_markdown ? (await api.previewBlog(loaded.body_markdown)).html : ''); form.value = loaded; await nextTick(); if(editor.value) editor.value.innerHTML = html || '' } catch(e) { toast.error(e.message) } }
async function save() { try { form.value.body_html = editor.value?.innerHTML || ''; form.value.body_markdown = ''; if(form.value.public_id) await api.updateBlogPost(form.value.public_id,form.value); else { const r=await api.createBlogPost(form.value); form.value.public_id=r.public_id } await load();toast.success('文章已保存') } catch(e) { toast.error(e.message) } }
async function remove(post) { if(!await confirm({title:'删除文章',message:`确定删除「${post.title}」？`,confirmText:'删除',danger:true})) return; try { await api.deleteBlogPost(post.public_id); if(form.value.public_id===post.public_id) reset(); await load(); toast.success('文章已删除') } catch(e) { toast.error(e.message) } }
function format(command, value) { editor.value?.focus(); document.execCommand(command, false, value) }
function insertLink() {
  const selection = window.getSelection()
  const range = selection?.rangeCount ? selection.getRangeAt(0).cloneRange() : null
  if(!range || range.collapsed || !editor.value?.contains(range.commonAncestorContainer)) { toast.error('请先在正文中选中要加链接的文字'); return }
  const url = window.prompt('请输入 HTTPS 链接或站内路径（以 / 开头）')
  if(url === null) return
  let external = false
  try { const parsed = new URL(url); external = parsed.protocol === 'https:' && !!parsed.hostname && !parsed.username && !parsed.password } catch (_) {}
  if(!external && !/^\/(?!\/)[^\s]*$/.test(url)) { toast.error('链接须以 https:// 或 / 开头'); return }
  editor.value?.focus()
  if(range) { selection.removeAllRanges(); selection.addRange(range) }
  document.execCommand('createLink', false, url)
}
function pastePlainText(event) { event.preventDefault(); document.execCommand('insertText', false, event.clipboardData?.getData('text/plain') || '') }
async function upload(event, cover) { const file=event.target.files?.[0]; if(!file)return; uploading.value=true; try { const image=await api.uploadBlogImage(file); if(cover) form.value.cover_path=image.path; else editor.value?.insertAdjacentHTML('beforeend', `<p><img src="${image.url}" alt="图片说明"></p>`); toast.success('图片已上传') } catch(e) { toast.error(e.message) } finally { uploading.value=false; event.target.value='' } }
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
        <button type="button" @mousedown.prevent @click="format('formatBlock','p')">正文</button>
        <button type="button" @mousedown.prevent @click="format('formatBlock','h2')">标题 2</button>
        <button type="button" @mousedown.prevent @click="format('formatBlock','h3')">标题 3</button>
        <button type="button" @mousedown.prevent @click="format('bold')"><strong>加粗</strong></button>
        <button type="button" @mousedown.prevent @click="format('italic')"><em>斜体</em></button>
        <button type="button" @mousedown.prevent @click="format('insertUnorderedList')">项目列表</button>
        <button type="button" @mousedown.prevent @click="format('insertOrderedList')">编号列表</button>
        <button type="button" @mousedown.prevent @click="insertLink">链接</button>
      </div>
      <div ref="editor" class="rich-editor" contenteditable="true" role="textbox" aria-label="文章正文" aria-multiline="true" data-placeholder="在这里直接输入文章正文…" @paste="pastePlainText"></div>
      <p class="muted editor-hint">选中文字后可加粗、设标题或插入链接；图片请使用下方上传入口。</p>
      <label>插入正文图片</label><input type="file" accept="image/jpeg,image/png,image/webp" :disabled="uploading" @change="upload($event,false)" />
      <p v-if="uploading" class="muted">正在上传并处理图片…</p>
      <label>状态</label><select v-model="form.status"><option value="draft">草稿</option><option value="active">发布</option></select>
      <div class="blog-actions"><button v-if="canManage" class="primary">保存文章</button></div>
    </form>
  </section>
</template>
<style scoped>
.blog-layout{display:grid;grid-template-columns:minmax(280px,1fr) minmax(0,2fr);align-items:start;gap:var(--sp-4)}
.blog-list h3,.blog-editor h3{margin:0 0 var(--sp-4);font-size:var(--fs-lg)}
.empty-list{padding:var(--sp-6) 0}
.blog-row{display:flex;align-items:center;gap:var(--sp-2);padding:var(--sp-3) 0;border-bottom:1px solid var(--border)}
.blog-row:last-child{border-bottom:0}
.blog-title{min-width:0;flex:1;border:0;box-shadow:none;background:transparent;text-align:left;font-weight:600;overflow-wrap:anywhere}
.blog-title:hover{color:var(--accent);background:var(--surface-2)}
.blog-actions{display:flex;gap:var(--sp-3);margin-top:var(--sp-6)}
.cover-preview{display:block;max-width:240px;max-height:160px;object-fit:cover;margin:var(--sp-2) 0;border-radius:var(--radius-md)}
.editor-hint{font-size:var(--fs-sm)}
.rich-toolbar{display:flex;flex-wrap:wrap;gap:var(--sp-2);padding:var(--sp-2);border:1px solid var(--border);border-bottom:0;border-radius:var(--radius-sm) var(--radius-sm) 0 0;background:var(--surface-2)}
.rich-toolbar button{padding:var(--sp-2)}
.rich-editor{min-height:340px;padding:var(--sp-4);border:1px solid var(--border);border-radius:0 0 var(--radius-sm) var(--radius-sm);background:var(--surface);line-height:1.7;overflow-wrap:anywhere;outline:none}
.rich-editor:focus{border-color:var(--accent)}
.rich-editor:empty:before{content:attr(data-placeholder);color:var(--muted);pointer-events:none}
.rich-editor :deep(img){max-width:100%;height:auto}
[inert]{opacity:.7}
@media(max-width:800px){.blog-layout{grid-template-columns:1fr}}
</style>
