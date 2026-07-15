# Kartwo — 决策日志（DECISIONS）

> 每个关键决策记一行：日期 / 决策 / 理由 / 影响范围。由 Claude Code 在每轮收尾追加维护。
> 作者：仗键天涯(daxing) ｜ 3442535897@qq.com

---

## 已定

| 日期 | 决策 | 理由 | 影响 |
|---|---|---|---|
| 2026-06-17 | 项目定名 **Kartwo**（kartwo.com） | 短、好拼、cart 语义、可注册 | 全局 |
| 2026-06-17 | Go 单二进制 + 内嵌 SQLite 默认、PostgreSQL 升级项 | 极简自部署、可移植、低资源 | 全局 |
| 2026-06-17 | 媒体本地 `./data/media` 默认、S3/R2 升级、CDN 前置 | 可移植 + 全球速度 | 媒体 |
| 2026-06-17 | 图片落盘 + DB 存相对路径（不存 BLOB），服务端处理（压缩/WebP/去 EXIF/内容哈希） | 性能/隐私/可移植/安全 | 媒体 |
| 2026-06-17 | StoragePolicy 可插拔：自部署不限额 / SaaS 按套餐配额 | 双模式分叉点 | 媒体/商业 |
| 2026-06-17 | 一套代码一个内核；开源 ⊂ 自部署商业 ⊂ SaaS，三形态层层包含 | 避免双重维护 | 全局 |
| 2026-06-17 | 支持自部署 + SaaS 双交付，内核单租户不变、控制面 v2+ | 形态/商业边界 | 全局 |
| 2026-06-17 | 加密密钥：自部署主口令派生（不随数据明文）、SaaS 平台托管 | 安全 vs 可移植折中 | 安全 |
| 2026-06-17 | 商业化走 Open Core，不做侵入式 license 锁 | 保护可移植与开源口碑 | 商业 |
| 2026-06-17 | 字体/图标仅用非商业开放许可，登记 LICENSES.md | 卖企业/开源合规 | 全局 |
| 2026-06-17 | 安全为贯穿全程硬约束 + CI 安全门禁（govulncheck+密钥扫描）无高危方可合主干 | 信任/合规 | 全局 |
| 2026-06-17 | 协作：Claude Code 规划+执行+git+文档一体；Derek 只做决策+人工测试 | 降低 Derek 负担 | 协作 |
| 2026-06-17 | 文档英文文件名（ARCHITECTURE/ROADMAP/...），版本交 git 管 | 开源友好 | 协作 |
| 2026-06-17 | 域名注册：Namecheap 注册 → 60 天后转入 Cloudflare Registrar | 成本价续费 + 免费 CDN/SSL/DNS/API | 基建 |
| 2026-06-17 | SaaS 二级域名自动化：通配符 DNS `*.kartwo.com` + 通配符证书 + Cloudflare API；后期商家自定义域名用 Cloudflare for SaaS | 自动 provision、内核不参与 | v2+/控制面 |
| 2026-06-17 | **数据层选型 = sqlc** | 最贴合"纯 SQL 迁移/禁 AutoMigrate/参数化/极简零运行时依赖"底线；SQLite+PG 双方言用 Repository 接口消化 | 数据层（M0 落地） |
| 2026-06-17 | Go 版本钉死 **1.26.4**（go.mod + CI 均写死，不用 latest） | 官方当前稳定版；可复现构建 | 全局/构建 |
| 2026-06-17 | SQLite 驱动用 **modernc.org/sqlite**（纯 Go，无 CGO） | 保证单静态二进制可交叉编译；契合零外部依赖 | 数据层 |
| 2026-06-17 | CI 安全门禁工具：govulncheck + **gitleaks**（密钥扫描）+ golangci-lint（含 gosec）；版本均钉死 | 无高危方可合主干 | CI/安全 |
| 2026-06-17 | 模块路径 `github.com/kartwo/kartwo`（暂定，未定远程仓库前的占位） | 需一个稳定 import 前缀；改动成本较低 | 全局 |
| 2026-06-17 | **v1 变体内核 = 双轴枚举变体（option × option，如尺码×颜色）**；不锁死具体品类 | 最通用形态，电子(颜色×容量)等双轴品类皆为子集 | 商品/变体（M1） |
| 2026-06-17 | 变体数据模型做成**通用 option/variant 结构**，不写死服装专用；demo 数据用服装(尺码×颜色)演示 | 通用、可扩展、零成本换演示品类 | 商品/变体（M1） |
| 2026-06-17 | **配置型/参数化商品（如眼镜"下单填度数"）推到 v1 之后，M1 不做** | 控范围、避免变体内核被定制项污染 | 范围（v1+ 后置） |
| 2026-06-17 | **customer/order schema 推迟到 M2/M3 使用时再建，M1 不留空表** | 避免镀金空表；schema 随行为一起落地 | 范围（M1→M2/M3） |
| 2026-06-17 | M1 切成 5 个独立可验收切片（数据模型/鉴权+向导/CRUD API/媒体+StoragePolicy/Admin SPA） | 遵守"每阶段十几分钟可人工测完"铁律 | 节奏（M1） |
| 2026-06-17 | 外部 ID 实现选 **UUIDv7**（google/uuid），不引 Hashid 盐管理；如需短公开码后续可叠加 | 时间有序、无需盐管理、落地简单（CONVENTIONS 许可二选一） | 数据模型 |
| 2026-06-17 | **sqlc(.sql) 注释用纯 ASCII、每条语句单行**，避开 sqlc v1.30 多字节注释致查询源跨度错位的 bug（生成可编译但 SQL 错乱、运行时报错） | 实测根因：注释含中文(多字节)时 sqlc 按字节偏移切片错位；纯 ASCII 注释生成正确；CI sqlc-diff 兜底 | 数据层/构建 |
| 2026-06-17 | **主口令 KDF = argon2id**（golang.org/x/crypto，无 CGO） | OWASP 首选、抗 GPU、纯 Go；CONVENTIONS 已列优先 | 安全/鉴权（M1.2） |
| 2026-06-17 | **argon2id 参数 memory=64MiB,time=3,parallelism=1,keyLen=32,saltLen=16** | 1C1G $5 VPS 友好、单次登录几十毫秒、安全充分（OWASP 推荐档） | 安全/鉴权（M1.2） |
| 2026-06-17 | **主口令与管理员登录密码合一**：一个口令既登录又派生配置加密密钥(KEK)，两用途用不同盐 | 非技术商家记一个口令最省心（北极星）；不同盐隔离两用途 | 安全/鉴权（M1.2） |
| 2026-06-18 | **图片 WebP 编码 = gen2brain/webp**（wazero 跑内嵌 libwebp WASM，无 CGO） | 无 CGO 下支持有损+无损 WebP、照片体积最优；仍单静态二进制；契合 ARCHITECTURE §5/§18 | 媒体（M1.4） |
| 2026-06-18 | 图片缩放用 **golang.org/x/image/draw**、WebP 解码 x/image/webp；去 EXIF 靠解码再编码(只留像素) | 纯 Go 无 CGO；高质量重采样；再编码天然剥 EXIF | 媒体（M1.4） |
| 2026-06-18 | 媒体存储后端做成 **Backend 接口 + LocalBackend 默认**；S3/R2 为 v1.1，仅留接口 | 双模式/可移植；本片只落地本地默认 | 媒体（M1.4） |
| 2026-06-18 | **Admin SPA 框架 = Vue 3 + Vite**；构建产物 embed 进单二进制，运行时零外部依赖；构建期需 Node | CRUD 样板最少/包体小/上手快，贴合极简；运行时仍单静态 | 前端/Admin（M1.5） |
| 2026-06-18 | **storefront v1 = 二进制内嵌默认主题**（独立 Next.js/Astro 留 v2+）；SEO(JSON-LD/sitemap/canonical/规范 meta)与页面性能做扎实，作产品卖点 | 单二进制零额外部署、贴合 30 分钟开店北极星；SEO/性能是迁站/获客命门 | 店面（M2） |
| 2026-06-18 | **Git 双远程推送**：origin fetch=GitHub，push 同时写 GitHub+Gitee（双备份） | 异地双备份、抗单点 | 基建 |
| 2026-06-18 | **店面 v1 用服务端渲染纯 HTML（Go html/template）**，购物车等交互用渐进增强少量 JS；Admin 维持 Vue SPA | SEO 卖点落地：HTML 对爬虫最友好、首屏最快、无 JS 也能下单；Admin 在登录后无需 SEO | 店面（M2） |
| 2026-06-18 | **v1 访客下单**（结账填邮箱即可），完整客户账户体系推迟 v1.1 | 北极星=快，强制注册流失；存最小客户信息即可 | 店面/订单（M2，账户 v1.1） |
| 2026-06-18 | **仓库 LICENSE 文件采用 MIT**（署名 2026 仗键天涯(daxing)），覆盖 GitHub 自动生成的 Apache-2.0 | 与"内核 MIT、Open Core"一致；MIT 最宽松主流，利开源引流+卖企业 | 全局/许可 |
| 2026-06-19 | **主攻市场 = 可扩展"市场框架"，v1 只点亮美国** | 支付/货币/语言/RTL/合规皆按市场配置；加新市场=插模块不返工；美国用 Stripe/PayPal/USD/en，无 RTL/i18n，销售税占位 | 全局/市场（M3） |
| 2026-06-19 | 向导加「选择主攻市场」步骤，文案大白话讲清 为何选/选了启用什么/可后调；不可用市场标"即将上线" | 非技术商家也懂、降低选择压力、正向信号 | 向导/市场（M3） |
| 2026-06-20 | **v1 店面(storefront)默认界面文案=英文**（因 v1 只点亮美国，面向美国顾客）；**完整多语言框架(商家自由切换/添加)仍在 v1.1**；Admin 后台保持中文 | 中文店面美国顾客看不懂=北极星打折；UI 文案改英文是小改、多语言框架才是大件后置 | 店面/i18n（v1 英文，框架 v1.1） |
| 2026-06-19 | 美国销售税 nexus v1 按简单常量/占位，完整税务后置 | 税务复杂、不在 M3 钻进去 | 合规/税务（v1+ 后置） |
| 2026-06-20 | **Stripe 集成用瘦 HTTP 客户端直连 API，不引官方 SDK** | 契合「单静态二进制/默认无外部依赖」；验签是安全命门，手写 raw-byte HMAC 比黑盒 SDK 更可审计、可单测；建会话/退款仅表单 POST | 支付（M3.2） |
| 2026-06-20 | **KEK-at-webhook = 方案 A**：登录解锁时把收款密钥解密入进程级内存缓存（绑定 KEK 金库：登录载入/登出销毁/改密钥即时重载），绝不落盘 | 不破坏「密钥加密存」铁律、对单租户自部署够用；代价=冷启动后需登录一次激活收款（诊断页兜底） | 支付/安全（M3.2） |
| 2026-06-20 | **密钥锁定时 Webhook 一律返回非 2xx（503）交 Stripe 重投；不自建事件 spool、不存未验签 payload** | 漏投全交网关重投机制兜底；杜绝「存了未验签原始数据」的攻击面与复杂度 | 支付/安全（M3.2） |
| 2026-06-20 | **Webhook 回调幂等：去重 INSERT（(provider,event_id) 唯一）与订单 pending→paid 在同一事务**（冲突即幂等返 2xx；pending 条件更新） | 杜绝「已标记已见过但未处理」丢单；状态机条件更新天然防重复副作用 | 支付/可靠性（M3.2） |
| 2026-06-20 | **Webhook 双校验**：①对原始字节 HMAC-SHA256 验签+时间戳容差防重放 ②比对订单号+金额+币种与库内订单一致；②前必先确认 session.payment_status=='paid'（不假设事件类型即已付） | 拒伪造/张冠李戴/篡改金额；按 ARCHITECTURE §9/§11 | 支付/安全（M3.2） |
| 2026-06-20 | **whsec 作后台收款页普通可编辑配置项**，明确区分本地 CLI 转发密钥与正式 Dashboard 端点密钥，均不写死 | 本地测试(stripe listen)与正式端点 whsec 不同，需可随时替换 | 支付（M3.2） |
| 2026-06-21 | **密钥来源=混合**：加密存后台为默认/主来源(§14 不变)，新增**可选 env 覆盖旁路**(`STRIPE_SECRET_KEY` 等)，优先级 env>加密库、**覆盖非双写**(不读库内值)、env 模式收款页只读、无锁定态、env 值不落库不进日志 | 给技术运维/本地联调一条 12-factor 路；普通商家仍走 UI 保北极星；单一来源杜绝跨源歧义；§14 仍权威、不收回任何决策 | 支付/安全（M3.2 增量） |
| 2026-06-21 | 上一条「密钥纪律=一律从 env 读」系表述错误，**已更正**：保密纪律(不硬编码/不进日志git/不回贴对话)不变，但密钥来源仍以加密存后台为默认 | 避免误拆已实现的加密存+方案A 设计 | 支付（澄清） |
| 2026-06-21 | **env 半设(有 secret 缺 whsec)启动 WARN 且绝不回退库取 whsec**；**env LIVE 密钥启动 WARN**；mode 推断按 `_live_` 段(兼容受限密钥 rk_) | 杜绝半回退歧义、防误上 LIVE、兼容 Stripe 推荐的 rk_ 受限密钥 | 支付/安全（M3.2 护栏） |
| 2026-06-21 | 经 `/stripe:stripe-best-practices` 复核：webhook 双校验**无偏离**官方推荐；Checkout Sessions(非 Charges)、不传 `payment_method_types`(启用动态支付方式)、推荐 rk_ 已采纳；IP allowlist 留 M5 硬化 | 锁定合规、记录有意未做项 | 支付（M3.2 复核） |
| 2026-06-22 | **M3.3 拆 3 小片**：3.3a 退款(Stripe) / 3.3b PayPal 沙箱 / 3.3c 向导支付步骤，各自独立验收 | 原 M3.3 三件大事超「十几分钟可测完」铁律 | 节奏（M3.3） |
| 2026-06-22 | **退款 v1 仅整单全额退款**，订单转 `refunded`；保留 `refund` 记录表结构，部分退款后续易加 | 落地最简、验收最快，不镀金 | 退款（M3.3a） |
| 2026-06-22 | **退款触发=新建最小后台订单页（列表+详情+退款按钮）**，鉴权+CSRF+对象级权限 | 后台此前无订单页，退款需人工入口 | 后台/退款（M3.3a） |
| 2026-06-22 | **退款依赖持久化 Stripe payment_intent**：在 `checkout.session.completed` 时存 payment_intent+provider 到订单/支付记录（退款退到 intent 非 session）；退款 webhook(`charge.refunded`)复用「双校验+同事务幂等」同步状态 | 不存 intent 则无法退款；状态同步防重复 | 退款（M3.3a） |
| 2026-06-22 | **PayPal 沙箱 webhook 本地用 webhook 模拟器验收**；真实端到端(顾客批准→真 webhook)推迟 M4(有 HTTPS/域名后) | PayPal 无 stripe listen 等价物、本地无公网 URL；不引隧道依赖(默认无外部依赖) | PayPal/验收（M3.3b/M4） |
| 2026-06-22 | **PayPal 与 Stripe 对称**：env 覆盖旁路(`PAYPAL_CLIENT_ID/SECRET`)、密钥加密存、hosted 审批+intent=CAPTURE(不做两段授权)、退款整数分、同事务幂等 | 一致性、北极星代码最少 | PayPal（M3.3b） |
| 2026-06-22 | 退款**先调网关、成功后才落库**(写 refund 记录 + paid→refunded 同事务)；`charge.refunded` webhook **只同步订单状态、不写退款记录**(记录由手动退款路径写，UNIQUE(provider_refund_id) 兜底) | 避免「库里已退钱没退」；避免跨源退款记录去重复杂度 | 退款（M3.3a 实现） |
| 2026-06-23 | **M3.3b 再拆 2 片**：3.3b-1 PayPal 付款(建单+审批+capture+结算支付方式选择UI，沙箱真跑付款) / 3.3b-2 PayPal 退款+webhook(模拟器验收) | 单片偏大、守「十几分钟可测完」铁律 | 节奏（M3.3b） |
| 2026-06-23 | **PayPal「已付」=同步 capture**：顾客批准跳回后我方主动 capture，响应 COMPLETED 即 paid(存 capture_id 作 payment_ref)；webhook 仅幂等补充同步 | capture 是 PayPal 权威同步信号；不卡在 webhook 验签限制上、happy-path 最稳 | PayPal（M3.3b-1） |
| 2026-06-23 | PayPal 金额 v1 仅 2 位小数币种(USD)：整数分↔小数字符串转换；多位/零位小数币种后置 | PayPal 用小数字符串、与内核整数分需转换；控范围 | PayPal（M3.3b-1） |
| 2026-06-23 | PayPal 全额退款=capture refund **空 body**(让 PayPal 退全部已捕获额)，忽略金额参数；复用既有退款编排，后台退款按钮对 PayPal 单即生效 | v1 仅全额、最简；无需传金额/币种 | PayPal（M3.3b-2） |
| 2026-06-23 | PayPal webhook **不走通用 VerifyWebhook 接口**(它单头、PayPal 需多头)，单独 `VerifyWebhookPayPal`(在线 verify-webhook-signature + webhook_id)；webhook_id 作明文配置项(+env PAYPAL_WEBHOOK_ID)；模拟器验收、真实验签 M4 | PayPal 验签是在线多头调用、与 Stripe 本地 HMAC 异构；模拟器过不了真实验签 | PayPal/webhook（M3.3b-2/M4） |
| 2026-06-23 | PayPal webhook 仅作**对账备份**：COMPLETED 备份改已付(happy-path 靠同步 capture)、REFUNDED 同步已退款(按 links 取 capture id)；不写退款记录(同 Stripe) | 付款/退款 happy-path 不依赖 webhook，webhook 只兜底外部改动 | PayPal/webhook（M3.3b-2） |
| 2026-06-24 | 向导「配置收款」步骤：needed=未配任何密钥 且 未跳过(`wizard.payment_skipped` 持久化)；配好或跳过后不再打扰，PaymentWizard 复用收款页组件 | 北极星引导但不强制；跳过持久化避免每次登录被烦 | 向导（M3.3c） |
| 2026-06-24 | 未付订单页「去支付」：**仅 pending 可重新发起**收款(复用结算跳转/方法选择)，已付/已退订单拒绝(防重复收款) | 顾客中途取消/弃单后能重付；状态守卫防重复扣款 | 店面/订单（M3.3c） |
| 2026-06-21 | **暂不钉 Stripe-Version，触发点=发版硬化(M4 前后)，修法=client 初始化显式钉死 API 版本**(请求加 `Stripe-Version` 头)。理由：Kartwo 分发到**不可控的商家账号**，各账号 Dashboard 默认 API 版本不同，不钉则同一份二进制在不同商家上行为可能漂移；M3.2 只读稳定字段(id/type/client_reference_id/payment_status/amount_total/currency)故当前安全。**仅记录，暂不改代码** | 可复现/抗版本漂移 vs 控范围 | 支付/硬化（M4 前后落地） |
| 2026-06-25 | **修复 PayPal capture 同步对账 bug**：capture 响应的 `custom_id` 在 `purchase_units[].payments.captures[].custom_id`（capture 对象）而非 purchase_unit 顶层；原解析只读顶层 → custom_id 恒空 → 对账判 mismatch → **钱已 capture 但订单不转 paid**（人工验收实测复现）。修法：解析器顶层缺失时回退取 capture 对象的 custom_id（两形态均覆盖）。同时给整条跳回/capture 链路补结构化日志（不记密钥），修正单测 mock 还原真实结构(custom_id 在 capture 对象)+新增顶层兼容护栏，红→绿坐实 | 静默失败+钱货不一致是支付严重 bug；测试 mock 失真才漏过 | 支付/PayPal（M3.3b-1） |
| 2026-07-05 | **观测性对齐**：退款成功路（手动+webhook 同步）与 Stripe webhook 落 paid 路补 INFO 结构化日志（provider/order_ref/refund_id 或 payment_ref/amount_cents，不记密钥）。此前 Stripe 成功路/退款成功路无 INFO，排障只能查 DB | 支付关键状态变更需可观测；Stripe/PayPal 两路对称 | 支付/观测性（M3 收官） |
| 2026-07-05 | **v0.3.0 前发现并修复 GO-2026-5061**：`golang.org/x/image` WebP 解码 panic DoS，govulncheck 判定被本仓调用（media 上传解码路径，管理员鉴权面、真实危害低）。**升 v0.42.0→v0.43.0**（纯补丁 bump），门禁复绿。**低危+零成本=直接修，不拖 M4、不开例外**——破例会侵蚀「无高危方可合主干」门禁纪律的先例价值 | 安全门禁是硬约束；机械补丁无理由拖延 | 安全/依赖（M3 收官） |
| 2026-07-05 | **验收数据教训**：M3 人工验收库曾放 `/tmp/kartwo_m3final_data`，被系统清理导致整库丢失（管理员+市场+凭证+订单+退款）不可恢复。此后**验收数据一律用持久路径 `~/kartwo-data`，严禁 /tmp、/private/tmp、/var/folders**；二进制也构建到项目内 `.bin/` 而非 /tmp。细则见 `docs/test/acceptance-data-dir.md`。验收沙箱库内订单（含弃单 `019f32bd`，TLS 抖动中断的 pending 残留）均为**测试夹具、非正式账目** | 易失目录放验收数据已致一次真实数据丢失 | 运维/验收流程 |
| 2026-07-06 | **M4.1 域名来源=env 覆盖 DB**：读"当前生效域名"先看 `KARTWO_DOMAIN`（非空即用、来源标 env、不读 DB），env 空才读 settings 的 `domain` 键（向导写入，M4.2）。不双写、不回退，与支付密钥 env 覆盖纪律完全同形 | 单一来源杜绝跨源歧义；12-factor 运维路 + 向导路并存 | HTTPS/配置（M4.1） |
| 2026-07-06 | **M4.1 autocert HostPolicy=单域名白名单**：只放行"当前生效域名"，其余 host 一律拒绝 | 防他人把域名解析到本机骗取证书、烧 Let's Encrypt 速率配额 | HTTPS/安全（M4.1） |
| 2026-07-06 | **M4.1 HTTP-only 评估态=一等受支持状态**：env 与 DB 都无域名时不启 TLS、于 `:80`（dev 仍 `:8080`）纯 HTTP 服应用，非 dev 专用回退。此态**严禁发 HSTS** | 全新安装未配域名时也要能开向导/店面；发了 HSTS 会把无证书店面锁死跳 HTTPS | HTTPS（M4.1） |
| 2026-07-06 | **M4.1 HSTS 门控改为按 TLS 实际启用**：原按 `Env==prod` 发，现改为仅"HTTPS 证书就位、真启用"时发（`securityHeaders(hstsEnabled)`）；prod 评估态/dev 均不发 | prod 评估态也是 HTTP，按 env 发 HSTS 会锁死；门控点从"环境"改为"是否真有 TLS" | HTTPS/安全（M4.1） |
| 2026-07-06 | **M4.1 证书缓存=KEK 加密宪法条文的显式例外**：TLS 证书用 autocert `DirCache` 落 `./data/certs`（目录 0700/文件 0600），**明文存放、绝不进 KEK 信封、启动无需主口令即可读**。理由：TLS 必须在任何人登录解锁 KEK 之前先起服，否则登录页本身打不开。此为"凭证一律 KEK 加密"铁律的唯一显式例外，特此记录避免将来被当漏洞误报。**M5 全量导出须排除 `certs/`**（证书可自动重签、不属可移植配置） | TLS 起服早于 KEK 解锁的时序硬约束 | HTTPS/安全（M4.1；导出 M5） |
| 2026-07-06 | **M4.1 ACME 目录 URL 可配**：`KARTWO_ACME_DIRECTORY` 默认空=Let's Encrypt 生产；可指 LE Staging 预跑验证（暴露 DNS/端口/防火墙真问题）而不烧生产配额。硬要求非可选 | 同一份二进制可先 staging 干跑再切生产，防配额浪费与误发 | HTTPS/运维（M4.1） |
| 2026-07-06 | **M4.1 端口**：prod 直绑 `:80`(ACME challenge + 301 跳 HTTPS) + `:443`(TLS)，可经 `KARTWO_HTTP_ADDR/KARTWO_HTTPS_ADDR` 覆盖；dev 保持 `:8080` 纯 HTTP。特权端口(<1024)绑定被拒时给人话提示(setcap/systemd/root/换高位端口)，不静默崩 | 单二进制自签发贴 $5 VPS；绑定失败要可诊断 | HTTPS/部署（M4.1） |
| 2026-07-06 | **改域名热切换不做顺滑化**：v1 容忍"改域名后重启一次进程"（生效域名在启动时解析一次定住）。低频运维动作，不为它镀金 | 控范围；autocert 动态 HostPolicy 的复杂度不值当 | HTTPS（M4.1，明确不做） |
| 2026-07-06 | **债1 PayPal webhook 真实端到端验签闭环了结（M4.1 真机验收）**：M3.3b-2 因模拟器过不了真实验签推迟至此，现于生产真证公网 HTTPS 上真机验收——真实 sandbox 付款→paid、退款→refunded，退款 webhook 带真实 `event_id`(WH-…) 进来经 `VerifyWebhookPayPal` 在线验签通过并同步订单状态，金额整数分实证。冒烟清单第 3 条已勾，不再"默认已验" | 长期挂账的真实验签命门需公网 HTTPS 才能验，M4.1 提供后即闭环 | 支付/PayPal（M4.1 债1 了结） |
| 2026-07-06 | **债2 Stripe-Version 钉死了结（选项 A：我方常量，不引 SDK）**：`stripe.go` 定义 `const stripeAPIVersion = "2026-06-24.dahlia"`，全部出站 Stripe 请求（Checkout 建单 + 退款）带 `Stripe-Version:<该常量>`；单一常量=单一事实源。**为何无 SDK 仍要钉**：不带头时 Stripe 按"账号当前默认 API 版本"塑形响应，该默认会被 Stripe 平台侧在我们不知情时推进，可能令我方手解的稳定字段(id/type/client_reference_id/payment_status/amount_total/currency)某天无编译错/无异常地静默错位；显式钉串=把响应结构从 Stripe 平台时间线拿回我方 git 时间线（此理由不依赖 SDK）。**版本选定依据**：Stripe 官方 upgrade skill 报当前稳定版=`2026-06-24.dahlia`；逐一核对 acacia→basil→clover→dahlia 四发布列车 changelog，我方六字段**无一受 breaking change 影响**（唯二相关项 dahlia 改 `ui_mode` 枚举、clover 移除 `currency_conversion` 均非我方所读字段）。**实跑验证**：本环境无 Stripe 测试密钥，无法自发真实请求；文档级核对已过，真实带头实跑并入 M4.1 段二 Stripe sandbox 验收（Derek 那笔真沙箱 Checkout+webhook 即真实带头验证——字段若变形订单不会转 paid，即时暴露）。**升级须走 CONVENTIONS 流程改此单点** | 可复现/抗版本漂移，不引 SDK 守单静态二进制宪法 | 支付/硬化（M4.1 债2 了结） |
| 2026-07-06 | **改价 UX=方案 A（每行价+量同存）**：编辑页变体表每行两个输入（价格·库存）+ 一个「保存」按钮，改哪行存哪行、价与量同存（升级原"存库存"）。后端为对称的两个端点(`/price`、`/inventory`)，前端一次保存顺序调用二者 | 改一行存一行最省点击、又不批量误改；不引入合并端点保持接口正交 | 商品/改价（M4.2 前置，已实现） |
| 2026-07-06 | **支持 0 价，但价格必填——空/缺失一律拒绝、绝不默认 0；显式 0 允许、负数拒；口径四处对齐**：前端创建、前端改价、后端创建、后端改价。**后端独立守**（DTO 用 `*int64` 区分缺失/空 nil→400 与显式 0），不依赖前端——防绕过前端不带价直调 API 致 0 价损失 | 免费/赠品需 0 价；但"忘填→默认 0→被 0 元下单"是真实损失，故必填是防损失底线、后端必须独立执行 | 商品/改价/安全（已实现） |
| 2026-07-06 | **CJK 竖排根治=对短控件统一 `white-space:nowrap`，不重构 `.row` 工具类**：根因是全局 `.row>*{flex:1}`(basis:0) 把窄 flex 子项压到 min-content、中文逐字断行；选按元素类型给 button/导航行子项/th/badge/chip/label 加 nowrap，而非改 `.row` 的 flex 数学（后者被多处合法表单行依赖、改动面大风险高）。正文/输入/描述不加保持换行 | 精准低风险、一次覆盖全部短控件；不动 flex 布局避免误伤表单 | 前端/Admin（M4.1 后小修） |
| 2026-07-07 | **M4.2.1 一气呵成外壳=D5-A（布尔链加进度 + 新步骤轻量回退，不重构已验收步骤）**：向导仍是 `App.vue` 的 `v-else-if` 布尔链，只加「第 X/N 步」进度条 + 新域名步「上一步」回收款步；**不重构 market/payment 已验收步骤、不引路由状态机、不允许回退已配市场**（域名 back 只把收款步重新打开，收款完成后再回域名步；market 无回退入口） | 低风险精准，不返工已验收步骤，与「一次只做当前范围/不镀金」一致；重构为路由状态机会动到已验收步骤、风险与工作量高 | 前端/向导（M4.2.1） |
| 2026-07-07 | **M4.2.1 向导进度计数口径**：`N` 固定=3（市场/收款/域名），**跳过的步骤仍占位、步号不跳变**，当前步号=所处步位置（市场=1、收款=2、域名=3） | 跳过步骤若不占位，步号会跳变（如跳过收款后域名从「第 3 步」变「第 2 步」）令非技术商家困惑；固定 N + 占位最稳定可预期 | 前端/向导（M4.2.1） |
| 2026-07-07 | **M4.2.1 域名步骤=D1-A（只写库 + 重启生效，不做热重载）**：向导录入域名→写 `settings.domain`（env `KARTWO_DOMAIN` 非空时后台只读、PUT 拒写 409，决策 C 不双写）；域名改动需**重启进程**由 autocert 签发/续期（重申 2026-07-06「域名热切换不做」，非新决策）。域名后端**独立校验**（拒空/协议前缀/路径/空格/非法字符/无点非 FQDN），不依赖前端。**dev 文案纪律**：dev（`Env!=prod`，`https_capable=false`）填域名只写库、绝不签发 HTTPS，向导须诚实标注不误导。**effectiveDomain 在 admin 内按 `server.EffectiveDomain` 同口径复刻**（admin 不能 import server 会成环），两处口径须一致 | 与既有 autocert/决策 C 纪律一致、最简；后端独立校验防绕过前端；dev 不签发须讲清防误解 | 前端/HTTPS/安全（M4.2.1） |
| 2026-07-07 | **M4.2.1 域名表单可复用**：`DomainSettings.vue` 同时供向导域名步（`DomainWizard.vue` 包裹）与后台 `/domain` 页复用（与 PaymentWizard 包裹 PaymentSettings 同构），后台导航加「域名」入口 | 跳过域名的商家事后仍需 UI 配域名（否则只能改 env 重启，非技术商家做不到），闭合北极星「先 HTTP 评估、买域名后再配」路径；与收款页对称 | 前端/向导（M4.2.1） |
| 2026-07-07 | **Admin 错误/成功提示采用"视口居中悬浮 toast"**：自写轻量机制（`toast.js`+`ToastHost.vue`，无外部库/图标资源，unicode ✓✕×）；error 红停留 6s+手动关、success 绿 3s、可堆叠、`position:fixed` 视口正中不随滚动、`pointer-events:none` 无遮罩不阻断页面。**本批只接改价/新建商品提示**，**全站其它页面提示统一迁此机制=M4.2** | 顶部文本提示在长表单底部操作时易被忽略("点保存像没反应")；视口居中最醒目、顶北极星非技术可用；折中范围先接高频改价路径不越界 | 前端/Admin（M4.1 后；全站迁移 M4.2） |
| 2026-07-15 | **安全补丁：Go 工具链 1.26.4→1.26.5，修 GO-2026-5856（`crypto/tls` ECH 隐私泄露）**：govulncheck 判定**真实可达**（autocert 服务端握手 + Stripe/PayPal 出站 HTTPS），Fixed in go1.26.5；升级后复扫 reachable 漏洞=0，门禁复绿。改 `go.mod`(`go 1.26.5`)+`ci.yml`(`GO_VERSION`)。**可复用原则（更新 2026-06-17 钉死决策）**：钉死版本**随官方安全补丁在同小版本内前移**属机械安全修复，照 2026-07-05（GO-2026-5061）先例**直接做、不开门禁例外、不必每次重拍**；跨小版本或有 breaking 的升级仍需单独决策。仍是钉死确定版本、可复现，与钉死精神一致。**留档不处理**：GO-2026-5932（`golang.org/x/crypto@v0.53.0`，Fixed in N/A、本仓**未调用**、govulncheck 判不可达=不门禁）。 | 无高危方可合主干是宪法级硬约束；可达 TLS 隐私漏洞不应挂主干；机械安全补丁照先例不拖不例外 | 全局/构建/安全 |
| 2026-07-15 | **M4.2.3a 全站提示迁移判据：瞬时事件→toast、持续状态→inline**：**瞬时事件**（用户主动操作后的一次性结果：保存/删除/退款/上传/选定的成功失败、"点了按钮但前置不满足"的动作级校验）→ 统一 `toast`（复用 error 6s+手动关 / success 3s）；**持续状态**（页级加载失败/404/空态占位/加载中/静态状态说明如「该订单已退款」）→ **保留 inline 常驻**（toast 会自动消失，持续错误一闪反而更差）。据此迁 8 处动作反馈（含 ProductList 删除成功新增「商品已删除」）、清理迁走后变死的 `msg`/`err` 残留、**精准保留** 6 处页级 load 失败 inline。**本片不碰**：`window.confirm()`（删除/退款，toast 无法替代阻塞式是/否决策）、`toast.js`/`ToastHost`（未发现需 warning 级，error/success 够用）、后端。**confirm 统一确认弹窗 + 页级错误视觉处理留 M4.2.3b** | 统一通知体验、顶北极星；区分瞬时/持续避免"持续错误一闪而过"；confirm 是阻塞决策非通知、需独立组件故后置 | 前端/Admin（M4.2.3a；confirm/美化 M4.2.3b） |
| 2026-07-08 | **M4.2.2 概览指标口径落定（D1–D7，全 SQL 聚合、只读无事务、单连接顺序、无 N+1）**：**D1 今日/近7日=服务器本地自然日**——今日=[本地今日零点,now]、近7日=[本地(今日-6天)零点,now]，在 Go 侧取本地零点(`time.Date(...,now.Location())`)再 `.UTC()` 按库内同格式(毫秒+Z)输出为下界，与 UTC 存储的 `created_at` 词法比较；此换算对"本地今日但 UTC 仍昨日"的订单仍正确计入今日（真机东八区复现证明，非 bug）；抽纯函数 `dashboardWindowBounds(now)` + `TestDashboardWindowBounds` 跨 UTC 日界确定性回归（固定时区+固定 now、CI UTC 稳定）防 false-green。按 `created_at`（下单时间，无 paid_at）分窗。**D2 待处理=`status='paid'` 计数**（已付待发货，不涉日期，全时）。**D3 库存告警**：可售=quantity−reserved，零(=0)/低(1..5，N=5 固定常量、v1 不可配)分列，软删变体/商品不计。**D6 销售额=`SUM(total_cents) WHERE status IN('paid','fulfilled')`**——refunded 单转 `refunded` 状态即离开该集合、**整额天然扣除且不依赖 refund 记录**（兼容 webhook 同步无记录的退款；v1 仅全额退款，部分退款是 v1 之后须改按 `refund.amount_cents` 扣减，已在代码注释标注）。**D4 加 `ix_order_created_at` 索引**（0010 迁移，面向订单表增长的廉价保险）。**D5 概览为登录默认落点**（`/`→`/dashboard`）。**D7 开店进度=三卡**（无商品/未配收款/未配域名）+全齐「开店就绪」，市场因向导强制先配不重复提示。空态诚实不造假数据、不堆图表 | 开店头几天真正要看的最小有用集；聚合在 SQL 防 N+1；本地日对自部署单租户最直觉；销售额按状态集合天然处理退款最稳且兼容 webhook 无记录场景 | 概览/订单/库存（M4.2.2） |

---

## 待定（未拍板前 Claude Code 不擅自选定）

| 决策 | 最迟需在 | 备注 |
|---|---|---|
| ~~债2 Stripe-Version 钉死的取值方式~~ | ~~M4.1 收尾~~ | ✅ **已了结（2026-07-06，选项 A）**：我方常量钉死 `2026-06-24.dahlia`、不引 SDK，见上表决策行 |
| ~~改价补全的 UX 补法~~ | ~~改价实现前~~ | ✅ **已定并实现（2026-07-06，方案 A 每行价+量同存）**，见上表决策行 |
| ~~是否支持 0 价（免费/赠品）~~ | ~~改价实现前~~ | ✅ **已定并实现（2026-07-06：支持 0 价但价格必填、空拒绝不默认 0、口径四处对齐）**，见上表决策行 |
