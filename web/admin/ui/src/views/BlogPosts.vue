<!-- 博客文章列表 / Blog Post List
功能：浏览文章并进入独立的新建、编辑页面
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-10-14 10:05:00 -->
<script setup>
import { ref, computed, inject, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'

const router = useRouter()
const toast = useToast()
const onUnauthorized = inject('onUnauthorized')
const demo = inject('demoSession', ref(false))
const role = inject('accountRole', ref(''))
const permissions = inject('accountPermissions', ref([]))
const canManage = computed(() => !demo.value && (role.value === 'owner' || permissions.value.includes('blog.manage')))
const posts = ref([])

async function load() {
  try { posts.value = (await api.listBlogPosts()).posts || [] }
  catch (e) { if (e instanceof APIError && e.status === 401) onUnauthorized(); else toast.error(e.message) }
}
async function remove(post) {
  if (!await confirm({ title: '删除文章', message: `确定删除「${post.title}」？`, confirmText: '删除', danger: true })) return
  try { await api.deleteBlogPost(post.public_id); await load(); toast.success('文章已删除') }
  catch (e) { toast.error(e.message) }
}
onMounted(load)
</script>
<template>
  <div v-if="canManage" class="page-toolbar page-toolbar--end"><button class="primary" @click="router.push('/blog/new')">新建文章</button></div>
  <p class="page-description">在此查看和管理文章，点击标题进入独立编辑页面。</p>
  <p v-if="demo" class="muted">公开演示中不显示真实文章草稿。</p>
  <section class="panel blog-list">
    <h3>文章列表</h3>
    <p v-if="!posts.length" class="muted empty-list">还没有文章。</p>
    <div v-for="post in posts" :key="post.public_id" class="blog-row">
      <RouterLink class="blog-title" :to="`/blog/${post.public_id}`">{{ post.title }}</RouterLink>
      <span class="chip" :class="{ draft: post.status !== 'active' }">{{ post.status === 'active' ? '已发布' : '草稿' }}</span>
      <button v-if="canManage" class="danger" @click="remove(post)">删除</button>
    </div>
  </section>
</template>
<style scoped>
.blog-list h3{margin:0 0 var(--sp-4);font-size:var(--fs-lg)}
.empty-list{padding:var(--sp-6) 0}
.blog-row{display:flex;align-items:center;gap:var(--sp-2);padding:var(--sp-3) 0;border-bottom:1px solid var(--border)}
.blog-row:last-child{border-bottom:0}
.blog-title{min-width:0;flex:1;color:var(--text);text-align:left;font-weight:600;overflow-wrap:anywhere;text-decoration:none}
.blog-title:hover{color:var(--accent);text-decoration:underline}
</style>
