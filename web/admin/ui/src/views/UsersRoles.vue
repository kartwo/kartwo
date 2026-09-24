<!-- 多用户与角色 / Users & Roles -->
<!-- 功能：店主管理员工、预设/自定义角色、一次性邀请与会话撤销 -->
<!-- 作者：仗键天涯(daxing) -->
<!-- 邮箱：3442535897@qq.com -->
<!-- 时间：2026-09-24 14:00:00 -->
<script setup>
import { onMounted, ref, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'

const toast = useToast()
const onUnauthorized = inject('onUnauthorized')
const users = ref([])
const roles = ref([])
const tab = ref('users')
const busy = ref(false)
const inviteURL = ref('')
const userForm = ref({ username: '', display_name: '', role_id: '' })
const roleForm = ref({ name: '', permissions: [] })
const permissionGroups = [
  ['概览与诊断', [['dashboard.view', '查看概览'], ['diagnostics.view', '查看诊断'], ['audit.view', '查看审计']]],
  ['商品与内容', [['products.view', '查看商品'], ['products.create', '新建商品'], ['products.edit', '编辑商品'], ['products.publish', '上下架商品'], ['products.delete', '删除商品'], ['products.inventory', '修改库存'], ['products.price', '修改价格'], ['products.media', '管理商品图片'], ['categories.manage', '管理分类'], ['content.manage', '管理内容页']]],
  ['订单', [['orders.view', '查看订单'], ['orders.customer_data', '查看客户信息'], ['orders.fulfill', '发货']]],
  ['其他', [['imports.manage', '导入商品'], ['settings.shop', '店铺设置'], ['settings.shipping', '配送设置'], ['settings.domain', '域名设置'], ['settings.policy', '政策设置']]],
]

async function load() {
  try {
    const [u, r] = await Promise.all([api.listUsers(), api.listRoles()])
    users.value = u.map(x => ({ ...x, _name: x.display_name, _role: x.role_id, _disabled: x.status === 'disabled' }))
    roles.value = r
    if (!userForm.value.role_id) userForm.value.role_id = r.find(x => x.system_key === 'viewer')?.public_id || r[0]?.public_id || ''
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  }
}
function fullInviteURL(path) { return new URL(path, window.location.origin).href }
async function copyInvite() {
  try { await navigator.clipboard.writeText(inviteURL.value); toast.success('邀请链接已复制') }
  catch (_) { toast.error('复制失败，请手动复制链接') }
}
async function createUser() {
  if (busy.value) return
  busy.value = true
  try {
    const result = await api.createUser(userForm.value)
    inviteURL.value = fullInviteURL(result.invite_path)
    userForm.value.username = ''; userForm.value.display_name = ''
    await load()
    toast.success('员工账号已创建；请安全地把一次性链接发给本人')
  } catch (e) { toast.error(e.message) } finally { busy.value = false }
}
async function saveUser(user) {
  try {
    await api.updateUser(user.public_id, { display_name: user._name, role_id: user._role, disabled: user._disabled })
    await load()
    toast.success('账号已更新，旧会话已撤销')
  } catch (e) { toast.error(e.message) }
}
async function resetPassword(user) {
  if (!await confirm({ title: '重置员工密码', message: `为 ${user.username} 生成新的 24 小时一次性链接？旧链接会失效。`, confirmText: '生成链接' })) return
  try {
    const result = await api.resetUserPassword(user.public_id)
    inviteURL.value = fullInviteURL(result.invite_path)
    toast.success('新链接已生成')
  } catch (e) { toast.error(e.message) }
}
async function revokeSessions(user) {
  if (!await confirm({ title: '撤销会话', message: `让 ${user.username} 的所有登录设备立即退出？`, confirmText: '撤销' })) return
  try { await api.revokeUserSessions(user.public_id); toast.success('会话已撤销') } catch (e) { toast.error(e.message) }
}
async function createRole() {
  try {
    await api.createRole(roleForm.value)
    roleForm.value = { name: '', permissions: [] }
    await load()
    toast.success('角色已创建')
  } catch (e) { toast.error(e.message) }
}
async function saveRole(role) {
  try { await api.updateRole(role.public_id, { name: role.name, permissions: role.permissions }); await load(); toast.success('角色已更新；相关员工需重新登录') }
  catch (e) { toast.error(e.message) }
}
async function deleteRole(role) {
  if (!await confirm({ title: '删除角色', message: `删除「${role.name}」？使用此角色的账号需先调整。`, confirmText: '删除', danger: true })) return
  try { await api.deleteRole(role.public_id); await load(); toast.success('角色已删除') } catch (e) { toast.error(e.message) }
}
onMounted(load)
</script>

<template>
  <h2>用户与角色</h2>
  <p class="muted">店主账号不可删除；员工密码与店铺加密主口令相互独立。邀请链接只显示这一次，请通过可信渠道发给本人。</p>
  <div class="tabs"><button :class="{ primary: tab === 'users' }" @click="tab = 'users'">用户</button><button :class="{ primary: tab === 'roles' }" @click="tab = 'roles'">角色与权限</button></div>
  <template v-if="tab === 'users'">
    <section class="panel card">
      <h3>邀请员工</h3>
      <div class="form-grid">
        <label>登录用户名（3–64 位英文、数字、.-_）<input v-model="userForm.username" maxlength="64" autocomplete="off" /></label>
        <label>显示名称<input v-model="userForm.display_name" maxlength="100" /></label>
        <label>角色<select v-model="userForm.role_id"><option v-for="role in roles" :key="role.public_id" :value="role.public_id">{{ role.name }}</option></select></label>
      </div>
      <button class="primary" :disabled="busy || !userForm.username || !userForm.role_id" @click="createUser">生成邀请链接</button>
      <div v-if="inviteURL" class="invite-box"><strong>一次性链接（24 小时有效）</strong><input :value="inviteURL" readonly aria-label="邀请链接" /><button @click="copyInvite">复制链接</button></div>
    </section>
    <section class="panel card"><h3>现有账号</h3>
      <div v-for="user in users" :key="user.public_id" class="entry">
        <div><strong>{{ user.username }}</strong><small class="muted">{{ user.status === 'invited' ? '待设置密码' : user.status === 'disabled' ? '已停用' : '正常' }} · 最近登录：{{ user.last_login_at || '从未登录' }}</small></div>
        <template v-if="user.role_id">
          <input v-model="user._name" aria-label="显示名称" placeholder="显示名称" maxlength="100" />
          <select v-model="user._role" aria-label="角色"><option v-for="role in roles" :key="role.public_id" :value="role.public_id">{{ role.name }}</option></select>
          <label class="check"><input v-model="user._disabled" type="checkbox" /> 停用</label>
          <button @click="saveUser(user)">保存</button><button @click="resetPassword(user)">重置密码</button><button @click="revokeSessions(user)">退出设备</button>
        </template>
        <span v-else class="muted">店主 · 全部权限</span>
      </div>
    </section>
  </template>
  <template v-else>
    <section class="panel card"><h3>新建自定义角色</h3><label>角色名称<input v-model="roleForm.name" maxlength="60" /></label>
      <div v-for="[group, permissions] in permissionGroups" :key="group" class="permission-group"><strong>{{ group }}</strong><div class="permission-grid"><label v-for="[key, label] in permissions" :key="key"><input v-model="roleForm.permissions" type="checkbox" :value="key" /> {{ label }}</label></div></div>
      <button class="primary" :disabled="!roleForm.name || !roleForm.permissions.length" @click="createRole">创建角色</button>
    </section>
    <section class="panel card"><h3>现有角色</h3><div v-for="role in roles" :key="role.public_id" class="role-entry"><div><strong>{{ role.name }}</strong><span class="muted"> · {{ role.user_count }} 位员工{{ role.system_key ? ' · 预设角色' : '' }}</span></div>
      <template v-if="!role.system_key"><label>名称<input v-model="role.name" maxlength="60" /></label><div v-for="[group, permissions] in permissionGroups" :key="group" class="permission-group"><strong>{{ group }}</strong><div class="permission-grid"><label v-for="[key, label] in permissions" :key="key"><input v-model="role.permissions" type="checkbox" :value="key" /> {{ label }}</label></div></div><div class="actions"><button @click="saveRole(role)">保存</button><button class="danger" @click="deleteRole(role)">删除</button></div></template>
      <p v-else class="muted">{{ role.permissions.length }} 项权限；预设角色不可编辑。</p></div>
    </section>
  </template>
</template>

<style scoped>
.tabs{display:flex;gap:var(--sp-2);margin:var(--sp-4) 0}.panel{margin-bottom:var(--sp-4)}.form-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--sp-3)}label{display:grid;gap:var(--sp-1)}.check{display:flex;align-items:center;white-space:nowrap}.check input{width:auto}.invite-box{display:grid;gap:var(--sp-2);margin-top:var(--sp-4);padding:var(--sp-3);background:var(--surface-2);border:1px solid var(--border);border-radius:var(--radius-md)}.entry{display:flex;flex-wrap:wrap;align-items:center;gap:var(--sp-2);padding:var(--sp-3) 0;border-top:1px solid var(--border)}.entry>div:first-child{min-width:12rem;margin-right:auto}.entry small{display:block}.entry input:not([type=checkbox]){max-width:11rem}.entry select{max-width:10rem}.role-entry{padding:var(--sp-4) 0;border-top:1px solid var(--border)}.role-entry>p{margin-bottom:0}.permission-group{margin:var(--sp-3) 0}.permission-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--sp-2);margin-top:var(--sp-2)}.permission-grid label{display:flex;align-items:center;font-weight:400}.permission-grid input{width:auto}.actions{display:flex;gap:var(--sp-2)}@media(max-width:760px){.form-grid,.permission-grid{grid-template-columns:1fr}.entry{align-items:stretch}.entry input,.entry select{max-width:none!important}}
</style>
