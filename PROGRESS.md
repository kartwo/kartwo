# Kartwo — 项目进度（PROGRESS）

> 项目进度的**单一事实来源**。Claude Code 每轮收尾必须更新此文件。
> 进度以本文件 + git tag 为准，不依赖对话记忆。
> 作者：仗键天涯(daxing) ｜ 3442535897@qq.com
> 最后更新：2026-07-15（M4.2.3a 全站提示迁移到 toast，人工验收通过，已合主干，不打 tag）

---

## 当前状态
- **阶段**：**M4.2（向导完整化 + Admin UI 完善）进行中**。已拍板 **M4.2 先于 M4.3**、M4.2 切片。
  - **M4.2.1 向导补完（域名步骤 + 一气呵成外壳）✅ 人工验收通过，已合主干**（分支 `feat/m4-domain-wizard` → `main`，**不打 tag**）。验收覆盖：主线三步连贯（第 X/3 步进度条）、域名录入 + dev 文案诚实（本地开发模式明确「不会真的签发 HTTPS」）、非法输入后端拦截（`http://`/路径/`localhost`/空格 → 拒）、域名步「上一步」回收款步且**退不回已配市场**、留空跳过持久化、店面 HTTP 评估态可访问；env 只读态以 curl 自测为准（`source=env`/`readonly=true`/PUT→409）。
  - **M4.2.2 dashboard 概览 ✅ 人工验收通过，已合主干**（分支 `feat/m4-dashboard` → `main`，**不打 tag**）。验收覆盖：空态诚实（0/友好占位）+ 开店进度三卡（无商品/未配收款/未配域名，可点击跳转）+ 随配置消长 + 全齐「开店就绪」；库存告警零/低分类正确（可售=quantity−reserved，低库存 3≤5 归低库存，N=5）；种子订单 o1–o4 实测：今日 3 / US$100.00（**refunded o3 的 $30 已扣除**，D6 命门验实）、近7日 4 / US$120.00、待处理 3（D2，refunded 不计）；概览登录默认落点。**D1 时区**曾疑似 bug，真机复现证明边界正确（"全 0"实为连到遗留旧实例、非 m422），并补 `TestDashboardWindowBounds` 跨 UTC 日界确定性回归测试锁死。
  - **M4.2.3a 全站提示迁移到 toast ✅ 人工验收通过，已合主干**（分支 `feat/m4-toast-migration` → `main`，**不打 tag**）。判据=瞬时事件（操作成功/失败/动作级校验）→ toast；持续状态（页级加载失败/404/空态/静态说明）→ 保留 inline。迁 8 处动作反馈（ProductList 删除成功「商品已删除」+失败、PaymentSettings 保存、PaymentWizard 跳过失败、MarketSelect 选定、ProductEdit 生成校验/基本信息/传图/删图、OrderDetail 退款）；清理死掉的 `msg`/`err` 残留；**保留** 6 处页级 load 失败 inline（验实「订单不存在」为常驻红字非一闪 toast）、confirm 两处未动、toast 机制未改、后端零改动。
  - **本片范围边界（明确不碰，留后续片）**：SMTP 全部并入 **M4.3**（与发信机器端到端做）；后台美化 + confirm 统一弹窗 + 页级错误视觉处理 = **M4.2.3b**；slug 自动、上传进度 = 后续体验片。
  - **附带安全补丁（本轮收尾时 govulncheck 新亮红灯）**：Go 工具链 **1.26.4→1.26.5** 修 **GO-2026-5856**（`crypto/tls` ECH 隐私泄露，真实可达）；独立 `chore(security)` commit；复扫 reachable 漏洞=0，门禁复绿。留档不处理 GO-2026-5932（x/crypto，无 fix、不可达不门禁）。详见 DECISIONS。
- **M4.1（自动 HTTPS + 债1 闭环 + 债2）✅ 已合主干**（不打 tag）。验收实证：①autocert LE Staging 预跑→生产真证浏览器绿锁、HSTS 门控正确、HTTP 301 保留路径、domain_source=env；②债1 PayPal webhook 真实端到端验签闭环（真付款→paid、真退款→refunded，退款 webhook 带真实 event_id `WH-9ML49990950259907-…` 同步，金额整数分实证）；③债2 Stripe-Version 钉死。
- **债1 PayPal webhook 真实验签**：✅ **已了结（2026-07-06 真机验收）**——M3.3b-2 推迟项闭环，冒烟清单第 3 条已勾。
- **债2 Stripe-Version 钉死**：✅ **已了结（2026-07-06，选项 A：我方常量 `2026-06-24.dahlia` 不引 SDK）**。
- **M4.1 后一批小修/补全（均已 Derek 验收合主干、不打 tag）**：① CJK 竖排 bug（`214cd58`+`e45d43f`）；② 商品改价缺口补全 + 0 价必填口径（`c9c9453`+`b503c2d`+`6578a96`）；③ 轻量 toast 通知机制 + 视口居中，先接改价/新建提示（`b2f0d82`+`01efe0d`）。
- **下一步**：**M4.2.3b**（Admin UI 整体美化 + confirm 统一确认弹窗 + 页级错误视觉处理）。之后 **M4.3**（SMTP 步骤 + 邮件队列 + 订单确认信）→ M4 收官（北极星计时打 `v0.4.0`）。散落待办（PayPal webhook INFO 日志、TLS 噪声日志治理、slug 自动、上传进度）按批统筹。
- **最新 git tag**：`v0.3.0`（M3）。M4.1、M4.2.1 及其间小修均已合主干，按切片纪律不单独打 tag。

## 里程碑总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 地基与骨架（含数据层选型落地、CI 安全门禁、生成各 .md） | ✅ 已验收通过（v0.0.0） |
| M1 | 核心数据模型 + Admin 基础 + 媒体上传 + StoragePolicy（切 5 片） | ✅ 已验收通过（v0.1.0） |
| M2 | 店面 + 购物车 + 下单（防超卖）+ SEO 基建（切 3 片） | ✅ 已验收通过（v0.2.0） |
| M3 | 支付路由 + Stripe/PayPal + 沙箱 + 退款 + 市场框架（切 3 片） | ✅ 已验收通过（v0.3.0） |
| M4 | 自动 HTTPS + 向导完整 + 30 分钟开店（北极星）**+ 承接：PayPal webhook 真实端到端验收（M3.3b-2 推迟项）** | 🟡 进行中（M4.1 ✅、M4.2.1 向导补完 ✅、M4.2.2 概览 ✅、M4.2.3a toast 迁移 ✅ 已合主干；M4.2.3b UI 美化 + M4.3 SMTP/邮件未开始；tag 待整体收官） |
| M5 | 数据导入(含301) + 诊断页 + 备份/导出/升级 | ⬜ 未开始 |
| M6 | v1.1 硬化（审计/签名/i18n/法律模板/Woo导入/S3）+ 验收 | ⬜ 未开始 |

> 状态图例：⬜ 未开始 ｜ 🟡 进行中 ｜ ✅ 已验收通过

## 里程碑明细（M4 · 进行中）
- [x] **M4.1 内嵌 autocert 自动 HTTPS + 债1 PayPal 真实验签闭环 + 债2 Stripe-Version 钉死**（✅ 2026-07-06 真机验收通过，已合主干，不打 tag）：
  - **自动 HTTPS**：prod 内嵌 autocert 自动签发/续期；域名来源 env 覆盖 DB(`KARTWO_DOMAIN`>settings.domain，不双写不回退，决策1 选 C)；HostPolicy 单域名白名单；HTTP-only 评估态(env/DB 皆无域名，一等受支持态)；HSTS 门控(仅 TLS 真启用时发，评估态严禁)；证书缓存 DirCache 落 `data/certs`(0700/0600 明文，KEK 铁律显式例外，导出排除)；ACME 目录可配(`KARTWO_ACME_DIRECTORY`，可指 LE Staging 预跑)；prod :80(challenge+301跳)/:443(TLS)，特权端口绑定被拒给 setcap/systemd/root 人话提示；单测(域名来源优先级/HostPolicy 白名单/HSTS 门控/证书目录 0700)
  - **债1（M3.3b-2 推迟项）闭环**：真机 LE Staging 预跑→生产真证浏览器绿锁；真实 sandbox PayPal 付款→paid、退款→refunded，退款 webhook 带真实 event_id 进来经 `VerifyWebhookPayPal` 真实在线验签通过并同步状态；金额整数分实证
  - **债2**：`stripe.go` 钉 `stripeAPIVersion="2026-06-24.dahlia"`，Checkout 建单+退款出站带 `Stripe-Version` 头；单测断言带头且值=常量；版本经官方 skill + 四发布列车 changelog 核对六字段无 breaking
  - **交付**：linux/amd64 交叉编译静态二进制 + Ubuntu 24.04 部署验收清单（scp 交付，非正式 release）
- [ ] **M4.2 向导完整化 + Admin UI 完善**（进行中，切片）：
  - [x] **M4.2.1 向导补完（域名步骤 + 一气呵成外壳）**（✅ 2026-07-07 人工验收通过，已合主干，不打 tag）：
    - **域名步骤**（向导第 3 步，收款后）：录入域名→前后端双校验→写 `settings.domain`；展示来源（env 只读/db/未配）；env 覆盖时只读、PUT 拒写 409（决策 C，不双写）；保存成功「需重启生效」醒目提示；**dev 文案纪律**（本地开发模式明确不签发 HTTPS）；留空跳过=复用原生 HTTP-only 评估态、持久化 `wizard.domain_skipped` 不再打扰
    - **一气呵成外壳（D5-A）**：布尔链加「第 X/3 步」进度指示（N 固定=3、跳过占位、步号不跳变）+ 域名步「上一步」轻量回收款步；**不重构已验收 market/payment、不引路由状态机、不回退已配市场**
    - **域名 D1-A**：只写库 + 重启生效，不做 autocert 热重载
    - **可复用**：`DomainSettings.vue` 被向导步与后台 `/domain` 页共用（与 PaymentSettings 同构），跳过者事后仍可从后台配域名
    - 单测：`validateDomain` 正反例、DB 路径 needed/存/校验/CSRF、skip、env 只读 409
  - [x] **M4.2.2 dashboard 概览首页**（✅ 2026-07-08 人工验收通过，已合主干，不打 tag）：
    - **统计卡（最小有用集）**：今日/近7日订单数+销售额、待处理数、商品数、库存告警（零/低）；金额整数分→按市场货币展示；全 SQL 聚合、只读无事务、单连接安全
    - **开店进度引导（D7）**：无商品/未配收款/未配域名 三张可点击卡 + 全齐「开店就绪」；空态诚实无假数据
    - **口径**：D1 今日/近7日=服务器本地自然日（本地零点正确换算 UTC 边界，跨 UTC 日界经回归测试锁死）；D2 待处理=`paid` 计数；D3 零(=0)/低(1..5，N=5 固定)；D6 销售额=`SUM(status IN paid,fulfilled)`，refunded 离开该集合而整额天然扣除（不依赖 refund 记录；部分退款是 v1 之后须改按 refund.amount_cents）；D4 加 `ix_order_created_at`；D5 概览为登录默认落点
    - 单测：`TestDashboard`（播种多状态订单/商品/库存断言聚合含 refunded 扣减）、`TestDashboardWindowBounds`（跨 UTC 日界确定性回归，CI UTC 稳定）、未登录 401
  - [x] **M4.2.3a 全站其它页面提示统一迁 toast**（✅ 2026-07-15 人工验收通过，已合主干，不打 tag）：判据「瞬时→toast、持续→inline」；迁 8 处动作反馈（含 ProductList 删除成功「商品已删除」新增）；清理死代码；保留 6 处页级 load 失败 inline + confirm 两处 + toast 机制未改；纯前端零后端改动
  - [ ] **M4.2.3b 后台整体美化 + confirm 统一确认弹窗 + 页级错误视觉处理**——未开始
- [ ] **M4.3 向导 SMTP 步骤 + 邮件队列**（SMTP 录入加密存 + 不阻塞下单 + 订单确认信 + 重试）——未开始

## 历史里程碑明细（M3 · 切 3 片，✅ v0.3.0）
- [x] **M3.1 市场框架 + 向导市场选择 + 加密设置地基**（✅ 已验收，含店面默认英文补丁）：可扩展 Market 注册表(US 点亮/其余即将上线)、AES-GCM(KEK)加密设置、向导市场步骤(大白话文案)、店面货币随市场；单测+实测
- [x] **M3.2 支付路由 + Stripe Checkout 沙箱 + Webhook 双校验（拒伪造/幂等）**（✅ 已验收，真实沙箱 A1~A3 通过）：PaymentProvider 抽象 + 瘦 Stripe 客户端(不引 SDK)；结算就绪即跳 Stripe 托管收银台、订单 public_id 作对账锚点；Webhook 双校验(原始字节 HMAC+时间戳防重放 + 订单号/金额/币种比对 + 显式 payment_status=='paid')；回调幂等(去重 INSERT 与 pending→paid 同一事务)；KEK 收款密钥内存缓存(登录解锁/登出销毁/改密钥即时重载)，锁定时 Webhook 返 503 交网关重投；后台收款页(sk/whsec 加密存)；**可选 env 覆盖旁路**(env>加密库/覆盖非双写/env模式收款页只读/不落库不进日志/记来源)；单测覆盖验签四态+双校验+幂等+缓存生命周期+env覆盖；实测 locked→503、env模式forged→400(不锁定)
- M3.3 PayPal 沙箱 + 退款(整数分) + 向导支付步骤 —— **拆 3 小片**（2026-06-22 拍板）：
  - [x] **M3.3a 退款(Stripe)**（✅ 已验收，真实沙箱退款通过）：迁移 0009(payment_provider/payment_ref 列 + refund 表)；webhook 落 payment_intent；后台手动整单全额退款(Stripe /v1/refunds，整数分，先退款后落库)；charge.refunded webhook 同步状态(双校验+同事务幂等)；订单状态 refunded；最小后台订单页(列表+详情+退款按钮)；单测(退款幸福路径/重复拒/未付拒/charge.refunded 幂等)；自驱实测(订单API/守卫409·404/charge.refunded→refunded)
  - M3.3b PayPal 沙箱 —— **再拆 2 片**（2026-06-23 拍板）：
    - [x] **M3.3b-1 PayPal 付款**（✅ 已验收，真实沙箱付款通过；含 capture 对账 custom_id 层级修复）：PayPalProvider(OAuth token/建单/同步 capture)；已付=capture COMPLETED+对账(custom_id/金额/币种)→pending->paid 落 capture_id；结算页支付方式选择(卡/PayPal，单个则隐藏)；/paypal/return 同步 capture；PayPal 密钥(client_id 明文/secret 加密)+收款页双区+**每通道独立 env 旁路**；金额 分↔小数串；单测(金额转换/AvailableMethods/建单/capture→paid/金额不符拒)；自驱实测(env来源/收款页/结算选择器渲染)
    - [x] **M3.3b-2 PayPal 退款 + webhook**（✅ 已验收，真实沙箱退款通过；webhook 真实验签留 M4）：capture 全额退款(空 body，复用退款编排，后台退款按钮对 PayPal 单生效)；PayPal webhook(/webhooks/paypal) 在线验签(verify-webhook-signature+webhook_id)+幂等，COMPLETED 备份同步/REFUNDED 状态同步；webhook_id 配置项(明文+env)；单测(退款/验签成败/COMPLETED 幂等/REFUNDED)；模拟器验收，真实端到端 M4
  - [x] **M3.3c 向导支付步骤 + 未付订单页「去支付」**（✅ 已验收，跳过路持久化实测通过）：开店向导加「配置收款」步骤(市场后、大白话引导、可跳过稍后配，跳过持久化不再打扰)；needed=未配且未跳过；PaymentWizard 复用收款页组件；未付订单页「Pay now」(按可用通道，仅 pending 可再发起、防对已付/已退重复收款)；单测(向导 needed/skip/CSRF、orderPay 仅 pending+Pay now 渲染)

### M3 收官补齐（v0.3.0 前已全部落地）
- [x] **退款成功路补 INFO 结构化日志**（手动退款 + 退款 webhook 同步：provider/order_ref/refund_id/amount_cents）——commit `8f88c24`。
- [x] **Stripe 成功路补 INFO 结构化日志，与 PayPal 观测性对齐**（`markPaid` webhook 落 paid）——commit `53a3831`。
- [x] **安全门禁修复**：合主干前 govulncheck 报 GO-2026-5061（x/image WebP 解码 DoS，管理员上传面，被本仓调用）→ 升 `golang.org/x/image v0.42.0→v0.43.0`，门禁复绿——commit `df90b5c`。

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
- [x] （M3 后）沙箱支付→订单已付→退款 可走（2026-07-05 通过：Stripe 全真跑；PayPal 付款/退款真跑，**webhook 真实验签除外**见下）✓
- [x] **（M4.1 后）PayPal webhook 真实端到端（公网 HTTPS + 真实 verify-webhook-signature）可走** —— ✅ 2026-07-06 真机验收通过（生产真证 + 真实退款 webhook `WH-9ML49990950259907-…` 验签通过并同步状态）；M3.3b-2 推迟项闭环
- [x] **（改价后）新建填价→创建；编辑页改价+改库存同存→生效；空价被拦(前端不提交、后端直调也 400)；0 价可存 可走**（2026-07-07 Derek 本机验收通过）
- [ ] （M5 后）Shopify CSV 导入→图本地化→301 生成 可走

---

## 待办登记（Derek 真机实测发现，按优先级；未拍板前不实现）
> 登记于 2026-07-06（M4.1 验收）。除高优先级缺口外，其余待 Derek 定 M4 后续范围时统筹排期。

- [x] **【已修复合主干】Admin SPA 全站 CJK 中文竖排 bug。** 根因=全局 `.row>*{flex:1}`(basis:0) 把窄 flex 子项压到 min-content、中文逐字断行；根治=对短控件(button/导航行子项/th/badge/chip/label)统一 `white-space:nowrap`，正文/输入/描述不动。commit `214cd58`(导航+订单表)+`e45d43f`(补全 button 等)，Derek 浏览器逐项目视验收通过，2026-07-06 合主干（不打 tag）。
- [x] **【✅ 已实现·已验收·已合主干】商品"创建后不能改价"缺口补全。** 价格为变体级。方案 A（每行价+量同存）：编辑页变体表价格改输入框、「保存」同存价+库存；后端新增 `SetVariantPrice`+`PATCH /variants/{id}/price`(鉴权+CSRF+对象级)+sqlc `UpdateVariantPrice`。**0 价口径**：支持 0 价但**价格必填、空/缺失拒绝、绝不默认 0**（DTO `*int64` 区分缺失 nil→400 与显式 0），四处对齐(前端创建/改价 + 后端创建/改价)，后端独立守防 0 价损失。新建矩阵价格框改 `type=text` 修默认显 0 坑。单测：`SetVariantPrice`(正/0/负/不存在) + HTTP 层缺价→400(创建+改价)。commit `c9c9453`+`b503c2d`+`6578a96`，Derek 本机全项验收通过，2026-07-07 合主干（不打 tag）。
- [x] **【✅ 已定并实现】是否支持 0 价（免费/赠品）。** 已定：**支持 0 价，但价格必填、空/缺失拒绝、绝不默认 0**（防"忘填→默认 0→0 元下单损失"），四处对齐、后端独立守。随改价补全一并落地合主干。
- [x] **【✅ 已实现·已验收·已合主干】轻量 toast 通知机制。** 视口居中悬浮、不随滚动、无遮罩不阻断；error 红 6s+手动关、success 绿 3s、可堆叠；unicode 图标无新增依赖/资源。**先接改价/新建商品提示**（新建校验错误、改价保存成功·失败）。commit `b2f0d82`(机制)+`01efe0d`(居中)，Derek 本机验收通过，2026-07-07 合主干。**全站其它页面提示统一迁此机制=M4.2**（见下）。
- [ ] **【可观测性】PayPal webhook 验签成功缺显式 INFO 日志。** 当前只有"已同步订单状态"隐含；建议补一条显式"验签通过"日志，与支付路径其他分支观测性对齐。
- [ ] **【可观测性·非紧急】公网 :443 TLS 握手噪声日志治理。**（M4.1 时漏登，补记）公网 443 被扫描器畸形探测 + HostPolicy 白名单拒绝骗签，产生大量 INFO 级 TLS handshake error，会吓到非技术商家并淹没业务日志。建议这类**外部握手失败降级 DEBUG/归类打标/限流采样**。顶"支持成本即生死线"；将来做。
- [ ] **【体验·不急】添加商品时 slug 从商品名自动生成（URL-safe）。** 当前需手填。
- [ ] **【体验·不急】上传图片时显示"上传中"进度反馈 + "上传成功"。** 当前无状态反馈；1C1G 上 WebP 编码耗时较长时尤其必要。

> **M4.2 范围确认在案**：Admin UI 完善含 ①dashboard/概览首页 ②后台整体设计打磨美化（需具体化设计方向）③**全站其它页面现有提示统一迁到 toast 机制**（本批只接了改价/新建，其余各页 err/msg 顶部文本待统一）。均非"开店赚钱底线"、归 M4.2。slug 自动生成、上传进度反馈等体验项亦可并入 M4.2 或单独体验批。

---

## 更新约定（Claude Code）
每轮收尾：① 更新里程碑状态与子任务勾选；② 更新"当前状态/下一步/最新 git tag"；③ 新决策同时写入 `DECISIONS.md`；④ 在回报中说明本文件改了什么。
