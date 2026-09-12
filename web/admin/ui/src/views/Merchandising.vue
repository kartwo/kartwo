<!-- 分类与内容 / Merchandising. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'

const toast = useToast(); const onUnauthorized = inject('onUnauthorized')
const pages = ref([])
const form = ref({ public_id:'', title:'', slug:'', body_markdown:'', seo_description:'', status:'draft' })
async function load() { try { pages.value=(await api.listContentPages()).pages||[] } catch(e) { if(e instanceof APIError&&e.status===401) onUnauthorized(); else toast.error(e.message) } }
async function edit(p) { try { form.value={...(await api.getContentPage(p.public_id))} } catch(e) { toast.error(e.message) } }
function reset() { form.value={ public_id:'', title:'', slug:'', body_markdown:'', seo_description:'', status:'draft' } }
async function save() { try { if(form.value.public_id) await api.updateContentPage(form.value.public_id,form.value); else { const r=await api.createContentPage(form.value);form.value.public_id=r.public_id };await load();toast.success('内容页已保存') } catch(e) { toast.error(e.message) } }
async function remove(p) { if(!await confirm({title:'删除内容页',message:`确定删除「${p.title}」？`,confirmText:'删除',danger:true}))return;try{await api.deleteContentPage(p.public_id);await load();if(form.value.public_id===p.public_id)reset();toast.success('内容页已删除')}catch(e){toast.error(e.message)} }
onMounted(load)
</script>
<template>
  <h2>内容页面</h2><p class="muted">内容页会显示在店面页脚。仅支持安全的 Markdown：标题（#）、列表（-）和普通段落；不执行 HTML。</p>
  <section class="split"><div class="panel"><h3>内容页</h3><button class="primary" @click="reset">新建内容页</button><p v-if="!pages.length" class="muted">尚无内容页。</p><div v-for="p in pages" :key="p.public_id" class="list-row"><button @click="edit(p)">{{p.title}}</button><span class="chip" :class="{draft:p.status==='draft'}">{{p.status==='active'?'已发布':'草稿'}}</span><button class="danger" @click="remove(p)">删除</button></div></div>
  <form class="panel" @submit.prevent="save"><h3>{{form.public_id?'编辑内容页':'新建内容页'}}</h3><label>标题</label><input v-model="form.title" required/><label>链接 slug</label><input v-model="form.slug" required :disabled="!!form.public_id"/><label>SEO 描述</label><textarea v-model="form.seo_description" rows="2"/><label>内容（安全 Markdown）</label><textarea v-model="form.body_markdown" rows="12"/><label>状态</label><select v-model="form.status"><option value="draft">草稿</option><option value="active">发布</option></select><div class="spacer"></div><button class="primary">保存内容页</button></form></section>
</template>
<style scoped>.split{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:var(--sp-4);margin-top:var(--sp-4)}.list-row{display:flex;align-items:center;gap:var(--sp-2);padding:var(--sp-2) 0;border-bottom:1px solid var(--border)}.list-row button:first-child{margin-right:auto}@media(max-width:720px){.split{grid-template-columns:1fr}}</style>
