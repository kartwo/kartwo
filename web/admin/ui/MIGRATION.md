# Admin React 迁移验收清单

作者：仗键天涯(daxing) · 3442535897@qq.com

## 交付条件

最终仅保留 React + TypeScript + Vite + Ant Design 后台，删除 Vue 运行时、旧页面及迁移专用构建配置。保留现有 `/admin/#/...` 地址、Go API 和单二进制 embed 托管。菜单权限仅优化界面体验，服务端鉴权仍是授权依据。

当前处于分批迁移：`src/react` 已实现请求传输、会话/登录/初始化/邀请、向导流程、导航外壳和路由访问控制；收款、域名和 SMTP 三个实际配置页已接入向导；店铺品牌、博客开关及客服留存设置也已迁移，账号安全、翻译和备份设置也已迁移，配送和店铺政策设置也已迁移，用户与角色管理也已迁移，十二个设置页均按需加载。`index.html` 仍连接现有后台，**未切换正式入口、未发布 React 后台**。新组件由 `build:react` 独立编译验证到忽略目录 `.react-build`，不进入 Go embed；这项临时验证在正式入口切换时移除。

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
| AccountSettings | React 已实现，待统一浏览器验收 | `getAccount`、`updateAccount`、`updateStaffPassword` |
| AuditEvents | 未迁移 | `auditEvents` |
| BackupSettings | React 已实现，待统一浏览器验收 | `getBackupSettings`、`setBackupSettings`、`testBackup` |
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
| PolicySettings | React 已实现，待统一浏览器验收 | `generateFooterPages`、`getPolicyProfile`、`setPolicyProfile` |
| ProductEdit | 未迁移 | `createProduct`、`deleteMedia`、`getProduct`、`getTranslation`、`listCategories`、`listMedia`、`setInventory`、`setPrice`、`translateText`、`updateMediaAlt`、`updateProduct`、`uploadMedia` |
| ProductList | 未迁移 | `deleteProduct`、`listProducts`、`setProductFeatured` |
| ShippingSettings | React 已实现，待统一浏览器验收 | `createShippingZone`、`deleteShippingZone`、`listShippingCountries`、`listShippingZones`、`saveDefaultShippingZone`、`saveShippingCountries`、`updateShippingZone` |
| ShopSettings | React 已实现，待统一浏览器验收 | `deleteShopLogo`、`getShop`、`setShop`、`uploadShopLogo` |
| SmtpSettings | React 已实现，待统一浏览器验收 | `getSmtp`、`setSmtp`、`smtpTest` |
| SmtpWizard | React 已实现，待统一浏览器验收 | `wizardSmtpSkip` |
| TranslationSettings | React 已实现，待统一浏览器验收 | `getTranslation`、`setTranslation` |
| UsersRoles | React 已实现，待统一浏览器验收 | `createRole`、`createUser`、`deleteRole`、`listRoles`、`listUsers`、`resetUserPassword`、`revokeUserSessions`、`updateRole`、`updateUser` |

## 已迁移配置页的验收边界

收款按 Stripe/PayPal 分别保存；密钥只提交、不回填，保存后清空输入。环境变量配置只读，店主仍可测试已保存配置；员工不可进入收款与邮件，域名保留独立权限，演示不可保存或测试。域名保存保留重启及本地不签发证书提示；SMTP 保留密码空白字符和留空不变语义。向导复用相同页面；读取失败不展示可编辑空配置，401 清除会话，正在加载或提交时不能跳到下一步。

店铺品牌保留授权员工可编辑和演示只读，名称与 Logo 共用操作锁，上传失败保留旧图，上传或删除 Logo 不覆盖未保存的名称。博客及客服设置店主可写、员工只读；开关与留存数值严格解析，读取失败不展示默认可保存表单，保存失败保留输入供重试。完整留言留存 1–24 个月、IP 留存 7–90 天保持原有规则。

账号设置保留店主修改用户名/可选主密码及员工只改自身密码；确认成功后立即清空会话，错误密码保留当前会话。翻译密钥不回填、空值保留，页面加载和保存均不自动调用翻译。备份保留本地策略重启生效、WebDAV 保存后测试及冷启动所需环境配置提示；环境只读字段从写入中省略，密码空值省略且空白原样保留，配置保存失败不发起远端测试，测试失败仍清空已经保存的密码。全环境配置允许店主仅测试、无需提交只读值。

配送保留按洲分组、中文/英文/代码搜索、范围全选/清空、默认及特殊运费、规则新增/编辑/确认删除；金额使用十进制字符串和 BigInt 精确转分，拒绝舍入/指数/溢出。只读账号可搜索和查看规则，编辑动作依据 settings.shipping。特殊规则只能选已保存启用国家；删除正在编辑的规则后重置编辑器。政策资料保留七洲时效，最大天数须不小于最小天数；资料编辑依据 settings.policy，生成页面额外需要 content.manage。生成前保存当前资料，覆盖正文前确认；取消不写入，保存失败不生成，生成失败明确已保存资料。现有草稿/发布保留语义继续由后端生成器负责。

用户与角色页面仅店主可访问；预设角色名称固定但权限可编辑，自定义角色可创建、改名、改权和删除，店主超管单独展示且无员工操作入口。角色依赖按现有服务端规则校验，用户/角色/会话/退款等店主专属权限不出现在可分配清单。邀请、停用/恢复、角色调整、密码重置和退出设备沿用原接口；重置、退出设备和删角色均先确认，仍有员工的角色须先调整。邀请链接严格限定本站固定入口与令牌格式，仅保留当前页面内存，支持复制/隐藏，跳页或会话过期清除。写入成功而列表刷新失败保留刚生成的邀请及成功结果，明确提示刷新核对，不误报写入失败或自动重发。

十二个页面已使用独立动态导入；其余页面及最终生产包体仍待迁移和审查。接口桩测试验证请求内容与界面结果，不代表已完成真实支付平台、SMTP 或浏览器布局验收。

## 验证方式

- `npm test`：原有 Node 测试 + React DOM、路由、会话和传输测试。
- `npm run typecheck`：严格 TypeScript 检查。
- `npm run build`：测试、新组件编译检查、当前正式入口构建。
- `npm audit --registry=https://registry.npmjs.org --audit-level=high`：包含开发依赖的安全门禁。

React DOM 测试使用隔离 API 桩，不修改验收数据，也不等于真实浏览器、SMTP 或支付平台验收。完整迁移后复用常驻验收库，不要求清空或重建商品与配置。
