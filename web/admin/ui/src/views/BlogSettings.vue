<!-- 博客设置 / Blog Settings
功能：店主即时开启或关闭店面博客，关闭时保留文章
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-10-13 10:00:00 -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
const enabled=ref(true)
const demo=inject('demoSession',ref(false))
const toast=useToast()
const onUnauthorized=inject('onUnauthorized')
async function load(){try{enabled.value=(await api.getBlogSettings()).enabled}catch(e){if(e instanceof APIError&&e.status===401)onUnauthorized();else toast.error(e.message)}}
async function save(){try{enabled.value=(await api.setBlogSettings(enabled.value)).enabled;toast.success('博客设置已保存')}catch(e){toast.error(e.message);await load()}}
onMounted(load)
</script>
<template><h2>博客设置</h2><div class="panel"><label class="blog-toggle"><input v-model="enabled" type="checkbox" :disabled="demo" /> 开启店面博客</label><p class="muted">默认开启。关闭后顾客无法访问博客，导航和站点地图不再显示；文章与草稿都会保留。</p><button v-if="!demo" class="primary" @click="save">保存设置</button></div></template>
<style scoped>.blog-toggle{display:flex;align-items:center;gap:var(--sp-3);font-weight:700;margin-bottom:var(--sp-3)}</style>
