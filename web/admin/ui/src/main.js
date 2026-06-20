// SPA 入口 / SPA Entry
// 功能：装配 Vue 应用与路由
// 作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-06-18 10:20:00
import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import ProductList from './views/ProductList.vue'
import ProductEdit from './views/ProductEdit.vue'
import MarketSelect from './views/MarketSelect.vue'
import PaymentSettings from './views/PaymentSettings.vue'
import './style.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/products' },
    { path: '/products', component: ProductList },
    { path: '/products/new', component: ProductEdit },
    { path: '/products/:id', component: ProductEdit, props: true },
    { path: '/market', component: MarketSelect },
    { path: '/payment', component: PaymentSettings },
  ],
})

createApp(App).use(router).mount('#app')
