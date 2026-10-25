# Admin React 迁移验收清单

作者：仗键天涯(daxing) · 3442535897@qq.com

## 交付条件

最终仅保留 React + TypeScript + Vite + Ant Design 后台，删除 Vue 运行时、旧页面及迁移专用构建配置。保留现有 `/admin/#/...` 地址、Go API 和单二进制 embed 托管。菜单权限仅优化界面体验，服务端鉴权仍是授权依据。

当前处于迁移第一批：`src/react` 已实现请求传输、会话/登录/初始化/邀请、向导流程、导航外壳和路由访问控制。`index.html` 仍连接现有后台，**未切换正式入口、未发布 React 后台**。新组件由 `build:react` 独立编译验证到忽略目录 `.react-build`，不进入 Go embed；这项临时验证在正式入口切换时移除。

## 共用能力

- [x] 严格 TypeScript、React 与 Ant Design 依赖及构建检查。
- [x] CSS token 共用；ConfigProvider 适配深蓝侧栏和暖色工作区。
- [x] Cookie 同源请求、CSRF、JSON/FormData、401/429/错误响应处理。
- [x] 初始化、登录、员工邀请表单及演示登录入口。
- [x] 会话失效、登出反馈及旧刷新不能覆盖新会话。
- [x] 三步向导编排、返回/跳过/失败提示；配置表单仍待下表迁移。
- [x] 全部旧业务路由清单和默认拒绝；博客独立一级菜单、移动端抽屉。
- [ ] 全部业务页面、数据契约、细粒度操作权限与错误/加载/空态。
- [ ] 富文本编辑器、图片上传、视频与内容兼容验证。
- [ ] 真实页面按路由惰性加载、懒加载失败恢复、生产包体审查。
- [ ] 新入口浏览器验收（键盘、窄屏、登录、店主/员工/演示、完整业务）。
- [ ] 切换唯一入口、移除 Vue 依赖及临时迁移构建。
- [ ] CI 安全门禁和常驻验收库人工回归。

## 现有页面与接口盘点

由旧页面实际 `api.*` 调用整理。页面迁移时还需检查组合子组件、下载直链、富文本行为和权限注入；接口列表不是完整交互验收的替代品。

| 原页面 | React 状态 | 原页面使用的 API |
|---|---|---|
| About | 未迁移 | 组合页面/静态内容，核对其子组件 |
| AccountSettings | 未迁移 | `getAccount`、`js`、`updateAccount`、`updateStaffPassword` |
| AuditEvents | 未迁移 | `auditEvents`、`js` |
| BackupSettings | 未迁移 | `getBackupSettings`、`js`、`setBackupSettings`、`testBackup` |
| BlogPostEdit | 未迁移 | `createBlogPost`、`getBlogPost`、`js`、`listBlogPosts`、`previewBlog`、`updateBlogPost`、`uploadBlogImage` |
| BlogPosts | 未迁移 | `deleteBlogPost`、`js`、`listBlogPosts` |
| BlogSettings | 未迁移 | `getBlogSettings`、`js`、`setBlogSettings` |
| CategoryManagement | 未迁移 | `createCategory`、`deleteCategory`、`js`、`listCategories`、`updateCategory` |
| ContactInbox | 未迁移 | `contactMessage`、`contactMessages`、`js`、`updateContactStatus` |
| ContactSettings | 未迁移 | `contactSettings`、`js`、`saveContactSettings` |
| Dashboard | 未迁移 | `dashboard`、`js` |
| Diagnostics | 未迁移 | `diagnostics`、`js` |
| DomainSettings | 未迁移 | `getDomain`、`js`、`setDomain` |
| DomainWizard | 未迁移 | `js`、`wizardDomainSkip` |
| ExportData | 未迁移 | 组合页面/静态内容，核对其子组件 |
| ImportCSV | 未迁移 | `executeImport`、`js`、`previewImportCSV` |
| Merchandising | 未迁移 | `createContentPage`、`deleteContentPage`、`getContentPage`、`js`、`listContentPages`、`updateContentPage` |
| OrderDetail | 未迁移 | `createRefund`、`fulfillOrder`、`getOrder`、`js`、`reconcileRefunds`、`refreshRefund`、`revokeOrderAccess` |
| OrderList | 未迁移 | `exportOrdersCSV`、`js`、`listOrders` |
| PaymentSettings | 未迁移 | `getPayment`、`js`、`setPayment`、`testStripeConnection` |
| PaymentWizard | 未迁移 | `js`、`wizardPaymentSkip` |
| PolicySettings | 未迁移 | `generateFooterPages`、`getPolicyProfile`、`js`、`setPolicyProfile` |
| ProductEdit | 未迁移 | `createProduct`、`deleteMedia`、`getProduct`、`getTranslation`、`js`、`listCategories`、`listMedia`、`setInventory`、`setPrice`、`translateText`、`updateMediaAlt`、`updateProduct`、`uploadMedia` |
| ProductList | 未迁移 | `deleteProduct`、`js`、`listProducts`、`setProductFeatured` |
| ShippingSettings | 未迁移 | `createShippingZone`、`deleteShippingZone`、`js`、`listShippingCountries`、`listShippingZones`、`saveDefaultShippingZone`、`saveShippingCountries`、`updateShippingZone` |
| ShopSettings | 未迁移 | `deleteShopLogo`、`getShop`、`js`、`setShop`、`uploadShopLogo` |
| SmtpSettings | 未迁移 | `getSmtp`、`js`、`setSmtp`、`smtpTest` |
| SmtpWizard | 未迁移 | `js`、`wizardSmtpSkip` |
| TranslationSettings | 未迁移 | `getTranslation`、`js`、`setTranslation` |
| UsersRoles | 未迁移 | `createRole`、`createUser`、`deleteRole`、`js`、`listRoles`、`listUsers`、`resetUserPassword`、`revokeUserSessions`、`updateRole`、`updateUser` |

## 验证方式

- `npm test`：原有 Node 测试 + React DOM、路由、会话和传输测试。
- `npm run typecheck`：严格 TypeScript 检查。
- `npm run build`：测试、新组件编译检查、当前正式入口构建。
- `npm audit --registry=https://registry.npmjs.org --audit-level=high`：包含开发依赖的安全门禁。

React DOM 测试使用隔离 API 桩，不修改验收数据，也不等于真实浏览器、SMTP 或支付平台验收。完整迁移后复用常驻验收库，不要求清空或重建商品与配置。
