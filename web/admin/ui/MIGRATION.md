# Admin React 迁移验收清单

作者：仗键天涯(daxing) · 3442535897@qq.com

## 交付条件

最终仅保留 React + TypeScript + Vite + Ant Design 后台，删除 Vue 运行时、旧页面及迁移专用构建配置。保留现有 `/admin/#/...` 地址、Go API 和单二进制 embed 托管。菜单权限仅优化界面体验，服务端鉴权仍是授权依据。

当前处于分批迁移：`src/react` 已实现请求传输、会话/登录/初始化/邀请、向导流程、导航外壳和路由访问控制；收款、域名和 SMTP 三个实际配置页已接入向导；店铺品牌、博客开关及客服留存设置也已迁移，六个设置页均按需加载。`index.html` 仍连接现有后台，**未切换正式入口、未发布 React 后台**。新组件由 `build:react` 独立编译验证到忽略目录 `.react-build`，不进入 Go embed；这项临时验证在正式入口切换时移除。

## 共用能力

- [x] 严格 TypeScript、React 与 Ant Design 依赖及构建检查。
- [x] CSS token 共用；ConfigProvider 适配深蓝侧栏和暖色工作区。
- [x] Cookie 同源请求、CSRF、JSON/FormData、401/429/错误响应处理。
- [x] 初始化、登录、员工邀请表单及演示登录入口。
- [x] 会话失效、登出反馈及旧刷新不能覆盖新会话。
- [x] 三步向导编排及实际配置表单、返回/跳过/失败提示；加载和保存期间禁止提前切页，明确区分已配置与稍后再配。
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
| AccountSettings | 未迁移 | `getAccount`、`updateAccount`、`updateStaffPassword` |
| AuditEvents | 未迁移 | `auditEvents` |
| BackupSettings | 未迁移 | `getBackupSettings`、`setBackupSettings`、`testBackup` |
| BlogPostEdit | 未迁移 | `createBlogPost`、`getBlogPost`、`listBlogPosts`、`previewBlog`、`updateBlogPost`、`uploadBlogImage` |
| BlogPosts | 未迁移 | `deleteBlogPost`、`listBlogPosts` |
| BlogSettings | React 已实现，待统一浏览器验收 | `getBlogSettings`、`setBlogSettings` |
| CategoryManagement | 未迁移 | `createCategory`、`deleteCategory`、`listCategories`、`updateCategory` |
| ContactInbox | 未迁移 | `contactMessage`、`contactMessages`、`updateContactStatus` |
| ContactSettings | React 已实现，待统一浏览器验收 | `contactSettings`、`saveContactSettings` |
| Dashboard | 未迁移 | `dashboard` |
| Diagnostics | 未迁移 | `diagnostics` |
| DomainSettings | React 已实现，待统一浏览器验收 | `getDomain`、`setDomain` |
| DomainWizard | React 已实现，待统一浏览器验收 | `wizardDomainSkip` |
| ExportData | 未迁移 | 组合页面/静态内容，核对其子组件 |
| ImportCSV | 未迁移 | `executeImport`、`previewImportCSV` |
| Merchandising | 未迁移 | `createContentPage`、`deleteContentPage`、`getContentPage`、`listContentPages`、`updateContentPage` |
| OrderDetail | 未迁移 | `createRefund`、`fulfillOrder`、`getOrder`、`reconcileRefunds`、`refreshRefund`、`revokeOrderAccess` |
| OrderList | 未迁移 | `exportOrdersCSV`、`listOrders` |
| PaymentSettings | React 已实现，待统一浏览器验收 | `getPayment`、`setPayment`、`testStripeConnection` |
| PaymentWizard | React 已实现，待统一浏览器验收 | `wizardPaymentSkip` |
| PolicySettings | 未迁移 | `generateFooterPages`、`getPolicyProfile`、`setPolicyProfile` |
| ProductEdit | 未迁移 | `createProduct`、`deleteMedia`、`getProduct`、`getTranslation`、`listCategories`、`listMedia`、`setInventory`、`setPrice`、`translateText`、`updateMediaAlt`、`updateProduct`、`uploadMedia` |
| ProductList | 未迁移 | `deleteProduct`、`listProducts`、`setProductFeatured` |
| ShippingSettings | 未迁移 | `createShippingZone`、`deleteShippingZone`、`listShippingCountries`、`listShippingZones`、`saveDefaultShippingZone`、`saveShippingCountries`、`updateShippingZone` |
| ShopSettings | React 已实现，待统一浏览器验收 | `deleteShopLogo`、`getShop`、`setShop`、`uploadShopLogo` |
| SmtpSettings | React 已实现，待统一浏览器验收 | `getSmtp`、`setSmtp`、`smtpTest` |
| SmtpWizard | React 已实现，待统一浏览器验收 | `wizardSmtpSkip` |
| TranslationSettings | 未迁移 | `getTranslation`、`setTranslation` |
| UsersRoles | 未迁移 | `createRole`、`createUser`、`deleteRole`、`listRoles`、`listUsers`、`resetUserPassword`、`revokeUserSessions`、`updateRole`、`updateUser` |

## 已迁移配置页的验收边界

收款按 Stripe/PayPal 分别保存；密钥只提交、不回填，保存后清空输入。环境变量配置只读，店主仍可测试已保存配置；员工不可进入收款与邮件，域名保留独立权限，演示不可保存或测试。域名保存保留重启及本地不签发证书提示；SMTP 保留密码空白字符和留空不变语义。向导复用相同页面；读取失败不展示可编辑空配置，401 清除会话，正在加载或提交时不能跳到下一步。

店铺品牌保留授权员工可编辑和演示只读，名称与 Logo 共用操作锁，上传失败保留旧图，上传或删除 Logo 不覆盖未保存的名称。博客及客服设置店主可写、员工只读；开关与留存数值严格解析，读取失败不展示默认可保存表单，保存失败保留输入供重试。完整留言留存 1–24 个月、IP 留存 7–90 天保持原有规则。

六个页面已使用独立动态导入；其余页面及最终生产包体仍待迁移和审查。接口桩测试验证请求内容与界面结果，不代表已完成真实支付平台、SMTP 或浏览器布局验收。

## 验证方式

- `npm test`：原有 Node 测试 + React DOM、路由、会话和传输测试。
- `npm run typecheck`：严格 TypeScript 检查。
- `npm run build`：测试、新组件编译检查、当前正式入口构建。
- `npm audit --registry=https://registry.npmjs.org --audit-level=high`：包含开发依赖的安全门禁。

React DOM 测试使用隔离 API 桩，不修改验收数据，也不等于真实浏览器、SMTP 或支付平台验收。完整迁移后复用常驻验收库，不要求清空或重建商品与配置。
