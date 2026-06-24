# Kartwo — 项目进度（PROGRESS）

> 项目进度的**单一事实来源**。Claude Code 每轮收尾必须更新此文件。
> 进度以本文件 + git tag 为准，不依赖对话记忆。
> 作者：仗键天涯(daxing) ｜ 3442535897@qq.com
> 最后更新：2026-06-17（M0 验收通过，已合主干打 v0.0.0）

---

## 当前状态
- **阶段**：**M3 全部切片代码就绪（M3.3b-1/b-2/c 待 Derek 一次性人工验收）**。
- **下一步**：Derek 重启回来、备好 PayPal 沙箱凭证后，一次性起服务，合并走完 M3.3b（Stripe 已过；PayPal 付款/退款）+ M3.3c（向导/去支付）人工验收 → M3 整片收官、合主干打 `v0.3.0`（PayPal webhook 真实验签按惯例推迟 M4，见回归冒烟清单）。
- **最新 git tag**：`v0.2.0`（M2）。

## 里程碑总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 地基与骨架（含数据层选型落地、CI 安全门禁、生成各 .md） | ✅ 已验收通过（v0.0.0） |
| M1 | 核心数据模型 + Admin 基础 + 媒体上传 + StoragePolicy（切 5 片） | ✅ 已验收通过（v0.1.0） |
| M2 | 店面 + 购物车 + 下单（防超卖）+ SEO 基建（切 3 片） | ✅ 已验收通过（v0.2.0） |
| M3 | 支付路由 + Stripe/PayPal + 沙箱 + 退款 + 市场框架（切 3 片） | 🟡 进行中（M3.1/M3.2/M3.3a 已验收；做 M3.3b） |
| M4 | 自动 HTTPS + 向导完整 + 30 分钟开店（北极星）**+ 承接：PayPal webhook 真实端到端验收（M3.3b-2 推迟项）** | ⬜ 未开始 |
| M5 | 数据导入(含301) + 诊断页 + 备份/导出/升级 | ⬜ 未开始 |
| M6 | v1.1 硬化（审计/签名/i18n/法律模板/Woo导入/S3）+ 验收 | ⬜ 未开始 |

> 状态图例：⬜ 未开始 ｜ 🟡 进行中 ｜ ✅ 已验收通过

## 当前里程碑明细（M3 · 切 3 片）
- [x] **M3.1 市场框架 + 向导市场选择 + 加密设置地基**（✅ 已验收，含店面默认英文补丁）：可扩展 Market 注册表(US 点亮/其余即将上线)、AES-GCM(KEK)加密设置、向导市场步骤(大白话文案)、店面货币随市场；单测+实测
- [x] **M3.2 支付路由 + Stripe Checkout 沙箱 + Webhook 双校验（拒伪造/幂等）**（✅ 已验收，真实沙箱 A1~A3 通过）：PaymentProvider 抽象 + 瘦 Stripe 客户端(不引 SDK)；结算就绪即跳 Stripe 托管收银台、订单 public_id 作对账锚点；Webhook 双校验(原始字节 HMAC+时间戳防重放 + 订单号/金额/币种比对 + 显式 payment_status=='paid')；回调幂等(去重 INSERT 与 pending→paid 同一事务)；KEK 收款密钥内存缓存(登录解锁/登出销毁/改密钥即时重载)，锁定时 Webhook 返 503 交网关重投；后台收款页(sk/whsec 加密存)；**可选 env 覆盖旁路**(env>加密库/覆盖非双写/env模式收款页只读/不落库不进日志/记来源)；单测覆盖验签四态+双校验+幂等+缓存生命周期+env覆盖；实测 locked→503、env模式forged→400(不锁定)
- M3.3 PayPal 沙箱 + 退款(整数分) + 向导支付步骤 —— **拆 3 小片**（2026-06-22 拍板）：
  - [x] **M3.3a 退款(Stripe)**（✅ 已验收，真实沙箱退款通过）：迁移 0009(payment_provider/payment_ref 列 + refund 表)；webhook 落 payment_intent；后台手动整单全额退款(Stripe /v1/refunds，整数分，先退款后落库)；charge.refunded webhook 同步状态(双校验+同事务幂等)；订单状态 refunded；最小后台订单页(列表+详情+退款按钮)；单测(退款幸福路径/重复拒/未付拒/charge.refunded 幂等)；自驱实测(订单API/守卫409·404/charge.refunded→refunded)
  - M3.3b PayPal 沙箱 —— **再拆 2 片**（2026-06-23 拍板）：
    - [x] **M3.3b-1 PayPal 付款**（待验收）：PayPalProvider(OAuth token/建单/同步 capture)；已付=capture COMPLETED+对账(custom_id/金额/币种)→pending->paid 落 capture_id；结算页支付方式选择(卡/PayPal，单个则隐藏)；/paypal/return 同步 capture；PayPal 密钥(client_id 明文/secret 加密)+收款页双区+**每通道独立 env 旁路**；金额 分↔小数串；单测(金额转换/AvailableMethods/建单/capture→paid/金额不符拒)；自驱实测(env来源/收款页/结算选择器渲染)
    - [x] **M3.3b-2 PayPal 退款 + webhook**（待验收）：capture 全额退款(空 body，复用退款编排，后台退款按钮对 PayPal 单生效)；PayPal webhook(/webhooks/paypal) 在线验签(verify-webhook-signature+webhook_id)+幂等，COMPLETED 备份同步/REFUNDED 状态同步；webhook_id 配置项(明文+env)；单测(退款/验签成败/COMPLETED 幂等/REFUNDED)；模拟器验收，真实端到端 M4
  - [x] **M3.3c 向导支付步骤 + 未付订单页「去支付」**（待验收）：开店向导加「配置收款」步骤(市场后、大白话引导、可跳过稍后配，跳过持久化不再打扰)；needed=未配且未跳过；PaymentWizard 复用收款页组件；未付订单页「Pay now」(按可用通道，仅 pending 可再发起、防对已付/已退重复收款)；单测(向导 needed/skip/CSRF、orderPay 仅 pending+Pay now 渲染)

## 历史里程碑明细（M2 · 切 3 片，✅ v0.2.0）
- [x] **M2.1 店面浏览 + 内嵌主题 + SEO 基建**（✅ 已验收）：SSR 目录/详情(Go template)、canonical/OG/JSON-LD(Product+AggregateOffer)、sitemap.xml/robots.txt、WebP 响应式图、Admin 迁至 /admin/、店面占 /；单测+HTTP 测+实测
- [x] **M2.2 购物车**（✅ 已验收）：匿名购物车(cookie/SameSite)、加/累加/改/删、购物车页+JSON、角标、渐进增强 cart.js、变体选择器；单测+实测（order/customer schema 按"用到才建"留 M2.3）
- [x] **M2.3 下单 + 库存预留防超卖**（✅ 已验收）：order/customer schema、结算表单(无JS可用)、订单确认页、原子预留防超卖、并发单测(库存5/并发20→恰好5成功)

## 历史里程碑明细（M1 · 切 5 片，✅ v0.1.0）
- [x] **M1.1 数据模型与迁移**（✅ 已验收）：通用双轴 option×option schema、纯 SQL 迁移、sqlc 数据层、seed-demo 装 6 变体并打印矩阵、单测
- [x] **M1.2 Admin 鉴权 + 向导骨架**（✅ 已验收）：argon2id 口令、主口令派生 KEK(内存金库)、初始化幂等、会话+CSRF、登录限流、向导 API；单测+HTTP 测
- [x] **M1.3 Admin 商品 CRUD API**（✅ 已验收）：商品建/列/取/改/软删、变体校验、改库存、分类增列；鉴权+对象级+CSRF；单测+HTTP 测+实测
- [x] **M1.4 媒体上传 + StoragePolicy**（✅ 已验收）：multipart 上传、magic-bytes、去 EXIF、多尺寸 WebP(gen2brain)、内容哈希、本地后端、StoragePolicy+磁盘护栏、单商品张数护栏、孤儿清理、/media 托管；单测+HTTP 测+实测
- [x] **M1.5 Admin SPA**（✅ 已验收=M1主验收通过）：Vue3+Vite 登录/向导、商品列表、新建(轴+变体矩阵)、编辑(基本信息/库存/传图预览)；embed 单二进制；CI 加前端构建

## 历史里程碑明细（M0 · 地基与骨架）
- [x] `git init` + Go module（`github.com/kartwo/kartwo`，go 1.26.4 钉死）
- [x] 目录骨架（cmd/internal/migrations/web）
- [x] 配置加载（env + 默认值，不读/记密钥）
- [x] 数据层选型落地 = sqlc（sqlc.yaml + 生成代码 + modernc.org/sqlite 驱动）
- [x] 纯 SQL 迁移框架（幂等可重入，禁 AutoMigrate）+ 示例迁移 `0001_meta`
- [x] 结构化日志（slog JSON）
- [x] HTTP server + 安全响应头中间件（CSP/X-Frame/X-Content-Type/Referrer，prod HSTS）
- [x] `/health` 健康检查（含 DB ping）
- [x] 内嵌 Admin SPA 占位页（embed 托管）
- [x] 优雅关停（SIGINT/SIGTERM）
- [x] 单元测试（迁移幂等/回滚、sqlc 全链路）
- [x] CI 安全门禁（go vet/test/build + golangci-lint + govulncheck + gitleaks，版本钉死）
- [x] 本地全部门禁绿（lint 0 issues、无高危漏洞、无密钥泄漏）

## 待决策（阻塞项）
- [x] ~~数据层选型 sqlc / ent / bun~~ → **已定 sqlc**（2026-06-17）
- [x] ~~第一类产品~~ → **已定：不锁品类，变体内核做双轴通用 option×option，demo 用服装**（2026-06-17）
- [ ] Admin SPA 框架 Vue3 / React —— 阻塞 M1 的 SPA 切片（数据模型/API 切片不受阻）
- [x] ~~主攻市场~~ → **已定：可扩展市场框架，v1 只点亮美国**（2026-06-19）
- [x] ~~storefront v1 形态~~ → **已定：二进制内嵌默认主题 + 扎实 SEO**（2026-06-18）

## 回归冒烟清单（每次合主干前 Derek 重跑，随功能增加）
- [x] （M2 后）开店→浏览→加购→下单 主干可走 ✓
- [ ] （M3 后）沙箱支付→订单已付→退款 可走（Stripe 全真跑；PayPal 付款/退款真跑，**webhook 真实验签除外**见下）
- [ ] **（M4 后）PayPal webhook 真实端到端（公网 HTTPS + 真实 verify-webhook-signature）可走** —— M3.3b-2 因模拟器过不了真实验签而推迟至此，**不得因 M3 收官打 tag 而默认为已验**
- [ ] （M5 后）Shopify CSV 导入→图本地化→301 生成 可走

---

## 更新约定（Claude Code）
每轮收尾：① 更新里程碑状态与子任务勾选；② 更新"当前状态/下一步/最新 git tag"；③ 新决策同时写入 `DECISIONS.md`；④ 在回报中说明本文件改了什么。
