<!-- 数据导出 / Data Export -->
<!-- 功能：下载包含数据库快照和媒体文件的一次性 ZIP 包 -->
<!-- 作者：仗键天涯(daxing) -->
<!-- 邮箱：3442535897@qq.com -->
<!-- 时间：2026-08-22 13:10:00 -->
<script setup>
function download() {
  window.location.assign('/admin/api/export')
}
</script>

<template>
  <div class="export-page">
    <h2>导出数据</h2>
    <section class="panel export-panel">
      <dl>
        <div><dt>数据库</dt><dd>SQLite 一致性快照</dd></div>
        <div><dt>媒体文件</dt><dd>包含</dd></div>
        <div><dt>证书缓存</dt><dd>不包含</dd></div>
      </dl>
      <button class="primary" @click="download">导出 ZIP</button>
    </section>
  </div>
</template>

<style scoped>
.export-page { display: grid; gap: var(--sp-4); }
.export-page h2 { margin: 0; }
.export-panel { display: grid; gap: var(--sp-4); }
dl { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: var(--sp-3); margin: 0; }
dl div { padding-top: var(--sp-2); border-top: 1px solid var(--border); }
dt { color: var(--text-muted); font-size: var(--fs-sm); }
dd { margin: var(--sp-1) 0 0; font-weight: 600; }
</style>
