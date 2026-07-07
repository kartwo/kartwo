<!-- 域名设置 / Domain Settings. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：读/写站点域名，展示来源(env 只读/db/未配)与「需重启生效」提示；向导域名步骤与后台域名页共用 -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const onUnauthorized = inject('onUnauthorized', null)
const toast = useToast()

const domain = ref('')          // 当前生效域名
const source = ref('none')      // env | db | none
const readonly = ref(false)     // env 覆盖 → 只读
const httpsCapable = ref(true)  // 本实例能否签发 HTTPS（prod=true，dev=false）
const input = ref('')
const busy = ref(false)
const saved = ref(false)        // 保存成功后展示「需重启生效」醒目提示

async function load() {
  try {
    const d = await api.getDomain()
    domain.value = d.domain || ''
    source.value = d.source
    readonly.value = !!d.readonly
    httpsCapable.value = !!d.https_capable
    input.value = source.value === 'db' ? domain.value : ''
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  }
}

async function save() {
  if (busy.value || readonly.value) return
  busy.value = true
  try {
    const d = await api.setDomain(input.value)
    domain.value = d.domain
    source.value = d.source
    saved.value = true
    toast.success('域名已保存')
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)  // 后端校验失败(400)/只读(409) 的具体原因在此提示
  } finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="domain-form">
    <!-- env 覆盖：只读 -->
    <template v-if="readonly">
      <p>当前域名：<strong>{{ domain }}</strong></p>
      <p class="muted" style="font-size:.9rem">
        该域名由环境变量 <code>KARTWO_DOMAIN</code> 提供，此处<strong>只读</strong>。如需修改，请改环境变量后重启进程。
      </p>
    </template>

    <!-- 可编辑 -->
    <template v-else>
      <label>你的店铺域名</label>
      <input v-model="input" placeholder="shop.example.com"
             autocapitalize="off" autocomplete="off" spellcheck="false" @keyup.enter="save" />
      <p class="muted" style="font-size:.85rem;max-width:64ch">
        只填域名本身，<strong>不要带 <code>http://</code> 或路径</strong>。填好后 Kartwo 会用它自动申请免费 HTTPS 证书（地址栏的安全绿锁）。
      </p>
      <p v-if="!httpsCapable" class="muted" style="font-size:.85rem;max-width:64ch">
        ⚠️ 当前是<strong>本地开发模式</strong>：填了域名也只会保存下来、<strong>不会真的签发 HTTPS</strong>（本地始终是 http://localhost）。
        正式部署（生产模式）重启后才会自动签发。
      </p>

      <div class="row" style="flex:0;gap:.6rem;margin-top:.5rem">
        <button class="primary" :disabled="busy" @click="save">保存域名</button>
      </div>

      <div v-if="saved" class="notice">
        <strong>✅ 域名已保存。</strong>
        <template v-if="httpsCapable">
          请<strong>重启一次 Kartwo 进程</strong>，之后会自动为 <strong>{{ domain }}</strong> 申请并启用 HTTPS。
          （首次签发需该域名已正确解析到本服务器、且 80 / 443 端口可从公网访问。）
        </template>
        <template v-else>
          当前为本地开发模式，<strong>不会签发 HTTPS</strong>；正式部署重启后才会生效。
        </template>
      </div>
    </template>
  </div>
</template>

<style scoped>
.domain-form input { width: 100%; }
.domain-form code { background: rgba(148,163,184,.18); padding: 0 .3rem; border-radius: 4px; font-size: .9em; }
.notice { margin-top: 1rem; border: 1px solid var(--accent); border-radius: 10px; padding: .8rem 1rem; line-height: 1.55; }
</style>
