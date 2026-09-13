<!-- 分类管理 / Category Management. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'

const toast = useToast()
const onUnauthorized = inject('onUnauthorized')
const demoSession = inject('demoSession', ref(false))
const categories = ref([])
const form = ref({ name: '', slug: '', position: 0 })
const busy = ref(false)

function slugify(value) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '')
}

async function load() {
  try {
    const data = await api.listCategories()
    categories.value = (data.categories || []).map(c => ({ ...c, _name: c.name, _slug: c.slug, _position: c.position }))
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  }
}

async function create() {
  busy.value = true
  try {
    await api.createCategory({ ...form.value, position: Number(form.value.position) || 0 })
    form.value = { name: '', slug: '', position: categories.value.length }
    await load()
    toast.success('分类已创建')
  } catch (e) { toast.error(e.message) } finally { busy.value = false }
}

async function save(category) {
  try {
    await api.updateCategory(category.public_id, { name: category._name, slug: category._slug, position: Number(category._position) || 0 })
    await load()
    toast.success('分类已保存')
  } catch (e) { toast.error(e.message) }
}

async function remove(category) {
  if (!await confirm({ title: '删除分类', message: `确定删除「${category.name}」？有关联商品的分类会被保护，不能直接删除。`, confirmText: '删除', danger: true })) return
  try {
    await api.deleteCategory(category.public_id)
    await load()
    toast.success('分类已删除')
  } catch (e) { toast.error(e.message) }
}

onMounted(load)
</script>

<template>
  <h2>分类管理</h2>
  <p class="muted">分类用于后台整理商品，也会按排序显示在英文店面导航和分类页。删除前须先把关联商品移到其他分类。</p>
	<p v-if="demoSession" class="demo-readonly">公开演示中分类为只读，临时商品仍可选择这些分类。</p>

	<form v-if="!demoSession" class="panel create-form" @submit.prevent="create">
    <h3>新建分类</h3>
    <div class="row">
      <div><label>分类名称</label><input v-model="form.name" required placeholder="例如 Running Tops" @input="form.slug ||= slugify(form.name)" /></div>
      <div><label>链接 slug</label><input v-model="form.slug" required placeholder="running-tops" /></div>
      <div class="position"><label>排序</label><input v-model.number="form.position" type="number" min="0" /></div>
      <button class="primary action" :disabled="busy">新建分类</button>
    </div>
  </form>

  <section class="panel category-list">
    <div class="list-head"><h3>现有分类</h3><span class="muted">共 {{ categories.length }} 个</span></div>
    <p v-if="!categories.length" class="muted">尚无分类，请先新建一个。</p>
		<div v-for="category in categories" :key="category.public_id" class="category-row" :class="{ 'demo-row': demoSession }">
			<div><label>名称</label><input v-model="category._name" :readonly="demoSession" /></div>
			<div><label>slug</label><input v-model="category._slug" :readonly="demoSession" /></div>
			<div class="position"><label>排序</label><input v-model.number="category._position" type="number" min="0" :readonly="demoSession" /></div>
      <div class="count"><span>{{ category.product_count }}</span><small>件商品</small></div>
			<template v-if="!demoSession"><button @click="save(category)">保存</button><button class="danger" @click="remove(category)">删除</button></template>
    </div>
  </section>
</template>

<style scoped>
.create-form{margin-bottom:var(--sp-4)}
.create-form .row{align-items:end}.create-form .action{flex:0;white-space:nowrap}
.position{max-width:7rem}.list-head{display:flex;align-items:center;justify-content:space-between}
.category-row{display:grid;grid-template-columns:minmax(12rem,1fr) minmax(12rem,1fr) 6rem 5rem auto auto;gap:var(--sp-2);align-items:end;padding:var(--sp-3) 0;border-bottom:1px solid var(--border)}
.category-row:last-child{border-bottom:0}.count{text-align:center;padding-bottom:.55rem}.count span{display:block;font-weight:800}.count small{color:var(--text-muted)}
.demo-readonly{padding:var(--sp-3);color:#713f12;background:#fef3c7;border:1px solid #f59e0b;border-radius:var(--radius-md)}
.demo-row{grid-template-columns:minmax(12rem,1fr) minmax(12rem,1fr) 6rem 5rem}.demo-row input{pointer-events:none;background:var(--surface-2)}
@media(max-width:900px){.category-row{grid-template-columns:1fr 1fr}.position{max-width:none}.count{text-align:left}.category-row button{width:100%}}
</style>
