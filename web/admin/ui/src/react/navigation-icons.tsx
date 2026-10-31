// 后台官方图标 / Official Admin Icons
// 功能：为既有路由及分组配置 Ant Design Icons，图标装饰化且不参与权限判断
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 14:52:16

import { AppstoreOutlined, AuditOutlined, BookOutlined, CarOutlined, CloudDownloadOutlined, CloudSyncOutlined, CreditCardOutlined, CustomerServiceOutlined, DashboardOutlined, DatabaseOutlined, FileTextOutlined, GlobalOutlined, ImportOutlined, InfoCircleOutlined, MailOutlined, SafetyOutlined, SettingOutlined, ShopOutlined, ShoppingCartOutlined, TagsOutlined, TeamOutlined, ToolOutlined, TranslationOutlined } from '@ant-design/icons'
import type { AdminRoute, Section } from './access'
const routeIcons: Record<string, typeof ShopOutlined> = {
  Dashboard: DashboardOutlined, OrderList: ShoppingCartOutlined, OrderDetail: ShoppingCartOutlined, ContactInbox: CustomerServiceOutlined,
  ProductList: ShopOutlined, ProductEdit: ShopOutlined, CategoryManagement: TagsOutlined, Merchandising: FileTextOutlined,
  BlogPosts: BookOutlined, BlogPostEdit: BookOutlined, ImportCSV: ImportOutlined, ExportData: CloudDownloadOutlined,
  Diagnostics: ToolOutlined, AuditEvents: AuditOutlined, About: InfoCircleOutlined,
  ShopSettings: ShopOutlined, PaymentSettings: CreditCardOutlined, DomainSettings: GlobalOutlined, SmtpSettings: MailOutlined,
  TranslationSettings: TranslationOutlined, BackupSettings: CloudSyncOutlined, ShippingSettings: CarOutlined, PolicySettings: FileTextOutlined,
  ContactSettings: CustomerServiceOutlined, BlogSettings: BookOutlined, AccountSettings: SafetyOutlined, UsersRoles: TeamOutlined,
}
const sectionIcons: Record<Section, typeof ShopOutlined> = { business: ShoppingCartOutlined, catalog: AppstoreOutlined, data: DatabaseOutlined, system: ToolOutlined, settings: SettingOutlined }
export function routeIcon(route: AdminRoute | undefined) { const Icon = route && routeIcons[route.source] || AppstoreOutlined; return <Icon aria-hidden /> }
export function sectionIcon(section: Section) { const Icon = sectionIcons[section]; return <Icon aria-hidden /> }
