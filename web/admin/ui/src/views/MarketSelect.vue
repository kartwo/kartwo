<!-- 选择主攻市场 / Market Selection. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const markets = ref([])
const currentCode = ref('')
const err = ref('') // 仅页级：市场列表加载失败的常驻错误（D2 保留 inline）
const busy = ref(false)

async function load() {
  err.value = ''
  try {
    const [ms, cur] = await Promise.all([api.markets(), api.getMarket()])
    markets.value = ms.markets || []
    if (cur.configured) currentCode.value = cur.current.code
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}

async function choose(m) {
  if (!m.available || busy.value) return
  busy.value = true
  try {
    await api.setMarket(m.code)
    currentCode.value = m.code
    toast.success('已设为「' + m.name + '」，店面已按该市场配置。')
    // 通知外层：市场已配置
    window.dispatchEvent(new CustomEvent('market-configured'))
  } catch (e) {
    toast.error(e.message)
  } finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="container">
    <h2>选择你的主攻市场</h2>
    <p class="muted" style="max-width:62ch">
      选定后，系统会自动配好该市场顾客最习惯的<strong>付款方式、货币和语言</strong>，让你的店面更贴合当地、更容易成交。
      不用担心选错——<strong>以后随时可以来这里调整或补充</strong>。
    </p>
    <p v-if="err" class="err">{{ err }}</p>

    <div class="market-grid">
      <div v-for="m in markets" :key="m.code"
           class="market-card"
           :class="{ active: m.code === currentCode, soon: !m.available }">
        <div class="mc-head">
          <strong>{{ m.name }}</strong>
          <span v-if="!m.available" class="chip">即将上线</span>
          <span v-else-if="m.code === currentCode" class="chip on">当前</span>
        </div>
        <p class="muted mc-enables">将启用：{{ m.enables }}</p>
        <p v-if="!m.available" class="muted" style="font-size:.82rem">{{ m.note }}</p>
        <button v-if="m.available" class="primary" :disabled="busy || m.code === currentCode" @click="choose(m)">
          {{ m.code === currentCode ? '已选用' : '选用这个市场' }}
        </button>
        <button v-else disabled title="持续上线中">敬请期待</button>
      </div>
    </div>

    <p class="muted" style="margin-top:1.2rem;font-size:.85rem">
      更多市场正在持续上线中。你可以<strong>先用美国开店赚钱</strong>，等你需要的市场上线后再一键切换。
    </p>
    <div class="spacer"></div>
    <button v-if="currentCode" @click="router.push('/products')">进入后台 →</button>
  </div>
</template>

<style scoped>
.market-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:1rem;margin-top:1rem}
.market-card{border:1px solid var(--line);border-radius:12px;padding:1rem;background:var(--panel)}
.market-card.active{border-color:var(--accent)}
.market-card.soon{opacity:.7}
.mc-head{display:flex;align-items:center;gap:.5rem;justify-content:space-between}
.mc-enables{font-size:.9rem;margin:.5rem 0}
.chip{font-size:.72rem;border:1px solid var(--line);border-radius:999px;padding:.05rem .5rem;color:var(--muted)}
.chip.on{color:#03263a;background:var(--accent);border-color:var(--accent)}
.market-card button{margin-top:.6rem;width:100%}
</style>
