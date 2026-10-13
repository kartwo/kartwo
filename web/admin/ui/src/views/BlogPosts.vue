<!-- 博客文章管理 / Blog Post Management
功能：草稿、预览、发布、封面和正文图片上传
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-10-13 10:00:00 -->
<script setup>
import { ref, computed, inject, onMounted } from 'vue'
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
const preview = ref('')
const uploading = ref(false)
function empty() { return { public_id:'', title:'', slug:'', excerpt:'', body_markdown:'', cover_path:'', seo_description:'', status:'draft' } }
async function load() { try { posts.value = (await api.listBlogPosts()).posts || [] } catch(e) { if(e instanceof APIError && e.status===401) onUnauthorized(); else toast.error(e.message) } }
function reset() { form.value = empty(); preview.value = '' }
async function edit(post) { try { form.value = await api.getBlogPost(post.public_id); preview.value = '' } catch(e) { toast.error(e.message) } }
async function save() { try { if(form.value.public_id) await api.updateBlogPost(form.value.public_id,form.value); else { const r=await api.createBlogPost(form.value); form.value.public_id=r.public_id } await load();toast.success('文章已保存') } catch(e) { toast.error(e.message) } }
async function remove(post) { if(!await confirm({title:'删除文章',message:`确定删除「${post.title}」？`,confirmText:'删除',danger:true})) return; try { await api.deleteBlogPost(post.public_id); if(form.value.public_id===post.public_id) reset(); await load(); toast.success('文章已删除') } catch(e) { toast.error(e.message) } }
async function showPreview() { try { preview.value=(await api.previewBlog(form.value.body_markdown)).html || '' } catch(e) { toast.error(e.message) } }
async function upload(event, cover) { const file=event.target.files?.[0]; if(!file)return; uploading.value=true; try { const image=await api.uploadBlogImage(file); if(cover) form.value.cover_path=image.path; else form.value.body_markdown += `\n![图片说明](${image.url})\n`; toast.success('图片已上传') } catch(e) { toast.error(e.message) } finally { uploading.value=false; event.target.value='' } }
onMounted(load)
</script>
<template>
  <h2>博客文章</h2><p class="muted">先保存草稿，确认内容后再发布。文章链接发布后保持不变。</p>
  <p v-if="demo" class="muted">公开演示中不显示真实文章草稿。</p>
  <section class="blog-layout"><div class="panel"><h3>文章</h3><button v-if="canManage" class="primary" @click="reset">新建文章</button><p v-if="!posts.length" class="muted">还没有文章。</p><div v-for="post in posts" :key="post.public_id" class="blog-row"><button @click="edit(post)">{{post.title}}</button><span class="chip">{{post.status==='active'?'已发布':'草稿'}}</span><button v-if="canManage" class="danger" @click="remove(post)">删除</button></div></div>
  <form class="panel" :inert="!canManage" @submit.prevent="save"><h3>{{form.public_id?'编辑文章':'新建文章'}}</h3><label>标题</label><input v-model="form.title" maxlength="160" required /><label>文章链接</label><input v-model="form.slug" pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="100" :disabled="!!form.public_id" placeholder="my-story" required /><label>摘要</label><textarea v-model="form.excerpt" maxlength="300" rows="3" /><label>SEO 描述</label><textarea v-model="form.seo_description" maxlength="300" rows="2" /><label>封面图</label><input type="file" accept="image/jpeg,image/png,image/webp" :disabled="uploading" @change="upload($event,true)" /><img v-if="form.cover_path" class="cover-preview" :src="'/media/'+form.cover_path" alt="封面预览" /><label>正文（安全 Markdown）</label><textarea v-model="form.body_markdown" rows="15" maxlength="100000" placeholder="## 小标题&#10;正文段落" /><p class="muted">支持标题、列表、链接和本地图片；不执行 HTML。</p><label>插入正文图片</label><input type="file" accept="image/jpeg,image/png,image/webp" :disabled="uploading" @change="upload($event,false)" /><p v-if="uploading" class="muted">正在上传并处理图片…</p><label>状态</label><select v-model="form.status"><option value="draft">草稿</option><option value="active">发布</option></select><div class="blog-actions"><button type="button" @click="showPreview">预览正文</button><button v-if="canManage" class="primary">保存文章</button></div><div v-if="preview" class="blog-preview" v-html="preview"></div></form></section>
</template>
<style scoped>.blog-layout{display:grid;grid-template-columns:minmax(280px,1fr) minmax(0,2fr);gap:var(--sp-4);margin-top:var(--sp-4)}.blog-row{display:flex;align-items:center;gap:var(--sp-2);padding:var(--sp-2) 0;border-bottom:1px solid var(--border)}.blog-row button:first-child{margin-right:auto}.blog-actions{display:flex;gap:var(--sp-3);margin-top:var(--sp-4)}.cover-preview{display:block;max-width:240px;max-height:160px;object-fit:cover;margin:var(--sp-2) 0}.blog-preview{border:1px solid var(--border);border-radius:var(--radius-sm);padding:var(--sp-4);margin-top:var(--sp-4);overflow-wrap:anywhere}.blog-preview :deep(img){max-width:100%;height:auto}[inert]{opacity:.7}@media(max-width:800px){.blog-layout{grid-template-columns:1fr}}</style>
