<!-- 商品列表 / Product List. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { useRouter } from 'vue-router'
import { api, APIError } from '../api.js'

const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const products = ref([])
const err = ref('')
const loading = ref(true)

async function load() {
  loading.value = true; err.value = ''
  try {
    const r = await api.listProducts()
    products.value = r.products || []
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  } finally { loading.value = false }
}

async function remove(p) {
  if (!confirm('确定删除商品「' + p.title + '」？')) return
  try {
    await api.deleteProduct(p.public_id)
    await load()
  } catch (e) { err.value = e.message }
}

onMounted(load)
</script>

<template>
  <div class="row" style="justify-content: space-between;">
    <h2>商品</h2>
    <button class="primary" style="flex:0" @click="router.push('/products/new')">+ 新建商品</button>
  </div>
  <p v-if="err" class="err">{{ err }}</p>
  <div class="panel">
    <p v-if="loading" class="muted">加载中…</p>
    <p v-else-if="!products.length" class="muted">还没有商品，点右上角「新建商品」。</p>
    <table v-else>
      <thead><tr><th>标题</th><th>slug</th><th>状态</th><th></th></tr></thead>
      <tbody>
        <tr v-for="p in products" :key="p.public_id">
          <td><RouterLink :to="'/products/' + p.public_id">{{ p.title }}</RouterLink></td>
          <td class="muted">{{ p.slug }}</td>
          <td><span class="chip">{{ p.status }}</span></td>
          <td style="text-align:right"><button class="danger" @click="remove(p)">删除</button></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
