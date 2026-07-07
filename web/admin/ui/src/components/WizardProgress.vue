<!-- 向导进度指示 / Wizard Progress. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：开店向导「第 X / N 步」进度条。N 固定=向导总步数，跳过的步骤仍占位显示，步号不跳变 -->
<script setup>
defineProps({
  step: { type: Number, required: true }, // 当前步号（1-based）
})
// 固定三步流：步号即位置，跳过的步骤仍占位（口径见 DECISIONS：N 固定、跳过占位、步号不跳变）。
const steps = ['选择市场', '配置收款', '配置域名']
</script>

<template>
  <div class="wiz-progress">
    <div class="wiz-count">第 {{ step }} / {{ steps.length }} 步</div>
    <ol class="wiz-steps">
      <li v-for="(name, i) in steps" :key="i"
          :class="{ done: i + 1 < step, active: i + 1 === step }">
        <span class="wiz-dot">{{ i + 1 < step ? '✓' : i + 1 }}</span>{{ name }}
      </li>
    </ol>
  </div>
</template>

<style scoped>
.wiz-progress { max-width: 880px; margin: 1rem auto 0; padding: 0 1.2rem; }
.wiz-count { font-size: .82rem; color: var(--muted); }
.wiz-steps { display: flex; gap: .5rem; list-style: none; padding: 0; margin: .4rem 0 0; flex-wrap: wrap; }
.wiz-steps li { display: flex; align-items: center; gap: .35rem; white-space: nowrap; font-size: .9rem;
  color: var(--muted); padding: .2rem .7rem; border: 1px solid var(--line); border-radius: 999px; }
.wiz-steps li.active { color: var(--text); border-color: var(--accent); }
.wiz-steps li.done { color: var(--ok); border-color: var(--ok); }
.wiz-dot { display: inline-flex; align-items: center; justify-content: center; width: 1.25rem; height: 1.25rem;
  border-radius: 50%; border: 1px solid currentColor; font-size: .72rem; }
</style>
