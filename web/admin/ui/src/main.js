// SPA 入口 / SPA Entry
// 功能：装配 Vue 应用与路由
// 作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-06-18 10:20:00
import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Dashboard from './views/Dashboard.vue'
import Diagnostics from './views/Diagnostics.vue'
import ExportData from './views/ExportData.vue'
import BackupSettings from './views/BackupSettings.vue'
import AuditEvents from './views/AuditEvents.vue'
import ProductList from './views/ProductList.vue'
import ProductEdit from './views/ProductEdit.vue'
import ImportCSV from './views/ImportCSV.vue'
import PaymentSettings from './views/PaymentSettings.vue'
import ShopSettings from './views/ShopSettings.vue'
import DomainSettings from './views/DomainSettings.vue'
import SmtpSettings from './views/SmtpSettings.vue'
import TranslationSettings from './views/TranslationSettings.vue'
import OrderList from './views/OrderList.vue'
import OrderDetail from './views/OrderDetail.vue'
import Merchandising from './views/Merchandising.vue'
import CategoryManagement from './views/CategoryManagement.vue'
import ShippingSettings from './views/ShippingSettings.vue'
import PolicySettings from './views/PolicySettings.vue'
import './style.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/dashboard' },
    { path: '/dashboard', component: Dashboard },
    { path: '/diagnostics', component: Diagnostics, meta: { demoReadonly: true } },
    { path: '/export', component: ExportData, meta: { demoReadonly: true } },
    { path: '/backup', component: BackupSettings, meta: { demoReadonly: true } },
    { path: '/audit', component: AuditEvents, meta: { demoReadonly: true } },
    { path: '/products', component: ProductList },
    { path: '/products/new', component: ProductEdit },
    { path: '/products/:id', component: ProductEdit, props: true },
    { path: '/imports/csv', component: ImportCSV, meta: { demoReadonly: true } },
    { path: '/market', redirect: '/payment' },
    { path: '/payment', component: PaymentSettings, meta: { demoReadonly: true } },
    { path: '/shop', component: ShopSettings, meta: { demoReadonly: true } },
    { path: '/domain', component: DomainSettings, meta: { demoReadonly: true } },
    { path: '/smtp', component: SmtpSettings, meta: { demoReadonly: true } },
    { path: '/translation', component: TranslationSettings, meta: { demoReadonly: true } },
    { path: '/orders', component: OrderList, meta: { demoReadonly: true } },
    { path: '/orders/:id', component: OrderDetail, props: true, meta: { demoReadonly: true } },
    { path: '/merchandising', component: Merchandising },
    { path: '/categories', component: CategoryManagement },
    { path: '/shipping', component: ShippingSettings, meta: { demoReadonly: true } },
    { path: '/policies', component: PolicySettings, meta: { demoReadonly: true } },
  ],
})

createApp(App).use(router).mount('#app')
