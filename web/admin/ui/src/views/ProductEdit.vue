<!-- 商品编辑 / Product Edit. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError } from '../api.js'

const props = defineProps({ id: { type: String, default: '' } })
const route = useRoute()
const router = useRouter()
const onUnauthorized = inject('onUnauthorized')

const isNew = computed(() => !props.id)
const err = ref('')
const msg = ref('')
const busy = ref(false)

// 公共字段
const title = ref('')
const slug = ref('')
const description = ref('')
const status = ref('draft')

// 新建：轴 + 生成的变体矩阵
const axes = ref([{ name: '尺码', valuesText: 'S, M, L' }, { name: '颜色', valuesText: '黑, 白' }])
const newVariants = ref([])

// 编辑：已存变体 + 图片
const variants = ref([])
const media = ref([])
const fileInput = ref(null)

function parseAxes() {
  return axes.value
    .map(a => ({ name: a.name.trim(), values: a.valuesText.split(',').map(s => s.trim()).filter(Boolean) }))
    .filter(a => a.name && a.values.length)
}

// 元(字符串)→整数分。价格必填：空/缺失/非法/负数一律返回 null（绝不静默当 0），
// 显式 0 返回 0（允许免费/赠品）。调用方 null 即拦下报错，不提交。
function yuanToCents(y) {
  if (y === '' || y === null || y === undefined) return null
  const n = Number(y)
  if (!Number.isFinite(n) || n < 0) return null
  return Math.round(n * 100)
}

function generateMatrix() {
  const parsed = parseAxes()
  if (!parsed.length) { err.value = '请先填写至少一个变体轴'; return }
  let combos = [[]]
  for (const ax of parsed) {
    const next = []
    for (const c of combos) for (const v of ax.values) next.push([...c, { option: ax.name, value: v }])
    combos = next
  }
  // 价格默认空（不是 0）：强制商家显式填写，杜绝"忘填→默认 0"。
  newVariants.value = combos.map(sel => ({ selections: sel, sku: '', priceYuan: '', quantity: 0 }))
  err.value = ''
}

async function saveNew() {
  if (!newVariants.value.length) { err.value = '请先「生成变体组合」'; return }
  // 价格必填校验：任一变体价格为空/非法 → 拦下不提交（可为 0，不能为空或负数）。
  const cents = []
  for (let i = 0; i < newVariants.value.length; i++) {
    const c = yuanToCents(newVariants.value[i].priceYuan)
    if (c === null) { err.value = `第 ${i + 1} 个变体：请填写有效价格（可为 0，不能为空或负数）`; return }
    cents.push(c)
  }
  busy.value = true; err.value = ''
  try {
    const payload = {
      title: title.value, slug: slug.value, description: description.value, status: status.value,
      options: parseAxes(),
      variants: newVariants.value.map((v, i) => ({
        sku: v.sku, price_cents: cents[i], quantity: Number(v.quantity),
        selections: v.selections,
      })),
    }
    const r = await api.createProduct(payload)
    router.push('/products/' + r.public_id)
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  } finally { busy.value = false }
}

async function load() {
  err.value = ''
  try {
    const d = await api.getProduct(props.id)
    title.value = d.title; slug.value = d.slug; description.value = d.description; status.value = d.status
    // _qty/_priceYuan 为编辑态输入绑定（价格以元回显，两位小数）。
    variants.value = d.variants.map(v => ({ ...v, _qty: v.quantity, _priceYuan: (v.price_cents / 100).toFixed(2) }))
    await loadMedia()
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}

async function loadMedia() {
  const r = await api.listMedia(props.id)
  media.value = r.media || []
}

async function saveFields() {
  busy.value = true; err.value = ''; msg.value = ''
  try {
    await api.updateProduct(props.id, { title: title.value, description: description.value, status: status.value })
    msg.value = '已保存'
  } catch (e) { err.value = e.message } finally { busy.value = false }
}

// saveVariant 同存该行的价格 + 库存（决策1A：改哪行存哪行、价+量同存）。
// 价格必填：空/非法拦下不发请求；库存拒负。
async function saveVariant(v) {
  err.value = ''; msg.value = ''
  const cents = yuanToCents(v._priceYuan)
  if (cents === null) { err.value = '价格必填：请填写有效价格（可为 0，不能为空或负数）'; return }
  const qty = Number(v._qty)
  if (!Number.isInteger(qty) || qty < 0) { err.value = '库存必须为非负整数'; return }
  try {
    await api.setPrice(v.public_id, cents)
    await api.setInventory(v.public_id, qty)
    v.price_cents = cents
    v.quantity = qty
    msg.value = '变体已保存（价格 + 库存）'
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}

async function onUpload(ev) {
  const file = ev.target.files && ev.target.files[0]
  if (!file) return
  busy.value = true; err.value = ''
  try {
    await api.uploadMedia(props.id, file)
    await loadMedia()
    msg.value = '图片已上传'
  } catch (e) { err.value = e.message } finally {
    busy.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

async function removeMedia(m) {
  try { await api.deleteMedia(m.public_id); await loadMedia() } catch (e) { err.value = e.message }
}

function thumb(m) {
  const d = (m.derivatives || []).find(x => x.label === 'thumb') || (m.derivatives || [])[0]
  return d ? d.url : m.original_url
}

onMounted(() => { if (!isNew.value) load() })
</script>

<template>
  <div class="row" style="justify-content: space-between;">
    <h2>{{ isNew ? '新建商品' : '编辑商品' }}</h2>
    <button style="flex:0" @click="router.push('/products')">← 返回</button>
  </div>
  <p v-if="err" class="err">{{ err }}</p>
  <p v-if="msg" class="ok">{{ msg }}</p>

  <div class="panel">
    <div class="row">
      <div><label>标题</label><input v-model="title" /></div>
      <div v-if="isNew"><label>slug（URL 标识，唯一）</label><input v-model="slug" /></div>
    </div>
    <label>描述</label>
    <textarea v-model="description" rows="2"></textarea>
    <label>状态</label>
    <select v-model="status">
      <option value="draft">草稿 draft</option>
      <option value="active">上架 active</option>
      <option value="archived">归档 archived</option>
    </select>
  </div>

  <div class="spacer"></div>

  <!-- 新建：轴 + 变体矩阵 -->
  <div v-if="isNew" class="panel">
    <h3>变体轴（如 尺码、颜色）</h3>
    <div v-for="(a, i) in axes" :key="i" class="row" style="margin-bottom:.5rem">
      <div><label>轴名</label><input v-model="a.name" /></div>
      <div style="flex:2"><label>取值（逗号分隔）</label><input v-model="a.valuesText" /></div>
      <button style="flex:0; align-self:end" @click="axes.splice(i,1)">删</button>
    </div>
    <div class="row" style="justify-content:flex-start">
      <button style="flex:0" @click="axes.push({name:'',valuesText:''})">+ 加轴</button>
      <button style="flex:0" @click="generateMatrix">生成变体组合</button>
    </div>

    <template v-if="newVariants.length">
      <h3>变体（{{ newVariants.length }} 个）</h3>
      <table>
        <thead><tr><th>组合</th><th>SKU</th><th>价格(元)</th><th>库存</th></tr></thead>
        <tbody>
          <tr v-for="(v, i) in newVariants" :key="i">
            <td>{{ v.selections.map(s => s.option + '=' + s.value).join(' × ') }}</td>
            <td><input v-model="v.sku" placeholder="可空" /></td>
            <td><input v-model="v.priceYuan" type="number" min="0" step="0.01" placeholder="元·必填" /></td>
            <td><input v-model="v.quantity" type="number" min="0" /></td>
          </tr>
        </tbody>
      </table>
    </template>

    <div class="spacer"></div>
    <button class="primary" :disabled="busy" @click="saveNew">保存商品</button>
  </div>

  <!-- 编辑：保存字段 + 变体库存 + 图片 -->
  <template v-else>
    <button class="primary" :disabled="busy" @click="saveFields">保存基本信息</button>

    <div class="spacer"></div>
    <div class="panel">
      <h3>变体与库存</h3>
      <table>
        <thead><tr><th>组合</th><th>SKU</th><th>价格(元)</th><th>库存</th><th></th></tr></thead>
        <tbody>
          <tr v-for="v in variants" :key="v.public_id">
            <td>{{ (v.options||[]).map(o => o.name + '=' + o.value).join(' × ') }}</td>
            <td class="muted">{{ v.sku || '—' }}</td>
            <td style="max-width:120px"><input v-model="v._priceYuan" type="number" min="0" step="0.01" placeholder="元·必填" /></td>
            <td style="max-width:120px"><input v-model="v._qty" type="number" min="0" /></td>
            <td style="text-align:right"><button @click="saveVariant(v)">保存</button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="spacer"></div>
    <div class="panel">
      <h3>图片</h3>
      <input ref="fileInput" type="file" accept="image/png,image/jpeg,image/webp" @change="onUpload" />
      <div class="thumbs">
        <figure v-for="m in media" :key="m.public_id">
          <img :src="thumb(m)" :alt="m.public_id" />
          <button class="danger" style="margin-top:.3rem; width:96px" @click="removeMedia(m)">删除</button>
        </figure>
      </div>
      <p v-if="!media.length" class="muted">还没有图片，选一张上传。</p>
    </div>
  </template>
</template>
