<!-- 商品列表 / Product List. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'
import ErrorState from '../components/ErrorState.vue'

const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const demoSession = inject('demoSession', ref(false))
const toast = useToast()
const products = ref([])
const err = ref('') // 仅页级：列表加载失败的常驻错误（D2 保留 inline）
const loading = ref(true)
const demoProductCount = computed(() => products.value.filter(p => p.demo_owned).length)

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
  const ok = await confirm({
    title: '删除商品',
    message: `确定删除商品「${p.title}」？此操作不可撤销。`,
    confirmText: '删除', danger: true,
  })
  if (!ok) return
  try {
    await api.deleteProduct(p.public_id)
    await load()
    toast.success('商品已删除')
  } catch (e) { toast.error(e.message) }
}

async function setFeatured(p) {
  try { await api.setProductFeatured(p.public_id, !p.featured); p.featured = !p.featured; toast.success(p.featured ? '已设为首页精选' : '已取消首页精选') } catch (e) { toast.error(e.message) }
}

// statusLabel 把状态值译成非技术商家看得懂的人话（草稿明确点出"店面看不到"）。
function statusLabel(s) {
  if (s === 'active') return '上架'
  if (s === 'archived') return '归档'
  return '草稿 · 店面看不到'
}

onMounted(load)
</script>

<template>
  <div class="row" style="justify-content: space-between;">
    <h2>商品</h2>
		<div style="flex:0;text-align:right">
			<button class="primary" :disabled="demoSession && demoProductCount >= 3" @click="router.push('/products/new')">+ 新建商品</button>
			<div v-if="demoSession" class="muted demo-quota">临时商品 {{ demoProductCount }} / 3</div>
		</div>
  </div>
  <ErrorState v-if="err" :message="err" :retry="load" />
  <div v-else class="panel">
    <p v-if="loading" class="muted">加载中…</p>
    <p v-else-if="!products.length" class="muted">还没有商品，点右上角「新建商品」。</p>
    <table v-else>
      <thead><tr><th>标题</th><th>slug</th><th>状态</th><th></th></tr></thead>
      <tbody>
        <tr v-for="p in products" :key="p.public_id">
					<td><RouterLink :to="'/products/' + p.public_id">{{ p.title }}</RouterLink> <span v-if="p.demo_owned" class="chip">我的临时商品</span></td>
          <td class="muted">{{ p.slug }}</td>
          <td><span class="chip" :class="{ draft: p.status === 'draft' }">{{ statusLabel(p.status) }}</span></td>
					<td style="text-align:right">
						<template v-if="!demoSession"><button :disabled="p.status !== 'active'" @click="setFeatured(p)">{{ p.featured ? '取消精选' : '设为精选' }}</button> <button class="danger" @click="remove(p)">删除</button></template>
						<button v-else-if="p.demo_owned" class="danger" @click="remove(p)">删除</button>
						<span v-else class="muted">只读</span>
					</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.demo-quota{margin-top:var(--sp-1);font-size:var(--fs-xs)}
</style>
