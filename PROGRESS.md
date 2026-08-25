# Kartwo — 项目进度（PROGRESS）

> 项目进度的**单一事实来源**。Claude Code 每轮收尾必须更新此文件。
> 进度以本文件 + git tag 为准，不依赖对话记忆。
> 作者：仗键天涯(daxing) ｜ 3442535897@qq.com
> 最后更新：2026-08-25（补充已真机验证的生产部署图文手册；M6.3 配置变更审计已人工验收合主干）

---

## 当前状态
- **文档**：新增生产部署图文手册 `docs/production-deployment.md`，基于 `www.kartwo.com` 的真机部署结果，将 README 中原有生产部署占位替换为可执行入口。手册覆盖 DNS/端口前置检查、专用低权限服务账户、systemd、内嵌 Let's Encrypt 自动 HTTPS、健康检查、备份与故障定位，并附 GitHub 可渲染流程图和录屏分镜；不记录密钥、口令或私人服务器信息。
- **阶段**：**M6.3 配置变更审计 ✅ 已人工验收合主干**（`PR #24` → `main`，`23ffbda`）。新增保存域名、SMTP 和市场三类成功配置操作的审计事件；均只记录固定设置域，SMTP 事件不包含密码。验收确认：恢复库以同值保存当前美国市场后，审计页显示 `admin` 的“更新市场设置”事件，已有审计历史仍完整。本片不纳入退款，退款需要独立的真实支付成功路径回归测试。
- **阶段**：**M6.2 关键后台操作审计 ✅ 已人工验收合主干**（`PR #22` → `main`，`1468293`）。在 M6.1 的只追加事件模型上，新增确认执行商品导入、保存自动备份设置和保存收款配置三类成功操作的事件；目标只标记为导入任务或固定设置域，绝不记录 CSV 内容、收款密钥或请求正文。验收确认：恢复库中保存原值 `90m / 12` 后，审计页显示 `admin` 的“更新自动备份设置”事件，既有商品事件仍保留；页面未显示敏感内容。
- **阶段**：**M6.1 后台审计日志 ✅ 已人工验收合主干**（`PR #20` → `main`，`0b4f236`）。新增只追加 `audit_event` 表及最近 100 条只读后台列表；记录登录、登出、商品创建/修改/删除、变体库存和价格改动。事件只含操作人、动作、稳定对象标识与时间，绝不存口令、密钥、会话令牌或请求正文；本片不做数据库外部篡改防护或历史事件导出。验收确认：在恢复库修改商品后，审计页显示 `admin` 的“更新商品”事件和 UTC 时间，页面未显示敏感内容。
- **阶段**：**M5.12 自动备份设置后台化 ✅ 已人工验收合主干**（`PR #18` → `main`，`ac6cc54`）。后台新增“备份”页，可保存自动备份周期（最小 `1m`）与保留份数（`1–365`）；保存值落库并于下次服务启动实际生效。`KARTWO_BACKUP_INTERVAL` / `KARTWO_BACKUP_RETENTION` 一旦显式提供即优先于数据库，并锁定后台对应字段为只读。升级快照始终独立于保留份数。验收确认：后台保存 `90m / 12` 后重启，日志显示 `interval="1h30m0s"`、`retention=12`，并成功生成自动备份。
- **阶段**：**M5.11 备份与升级快照诊断 ✅ 已人工验收合主干**（`PR #16` → `main`，`ae7be5a`）。后台“诊断”页新增本地保护点视图：仅统计程序生成的 `kartwo-backup-*.zip` 和 `kartwo-upgrade-*.zip`，展示自动备份数、升级快照数、总占用和最近生成时间；目录不存在按“尚未生成”呈现，读取异常明确提示而不伪造数字。验收确认：恢复库命令行与诊断页一致显示自动备份 5 份、升级快照 1 份、总占用 138 KB。
- **阶段**：**M5.10 升级快照与原子迁移 ✅ 已人工验收合主干**（`PR #14` → `main`，`6b303e6`）。仅在已有 `shop.db` 且存在待执行迁移时，启动期先生成不受日常保留策略清理的 `backups/kartwo-upgrade-<UTC>.zip` 全量快照；随后将本次全部待迁移 SQL 放在一个 SQLite 事务内执行，任一失败即整批回滚并拒绝启动。首次安装不创建空快照。验收确认：首次启动生成升级快照并应用 `0015`，修复后二次启动显示 `newly_applied=0`、本地自动备份完成且无 panic。
- **阶段**：**M5.9 WebDAV 异地备份 🟡 开发完成，待真实目标验收**（草稿 `PR #13`）。实现与 CI 已完成；由于尚未配置可写 HTTPS WebDAV 账号，未将协议单测替代为人工验收，也尚未合并。
- **阶段**：**M5.8 本地自动备份 ✅ 已人工验收合主干**（`PR #11` → `main`）。服务启动后异步创建一次全量 ZIP 备份，之后默认每 24 小时继续执行；`KARTWO_BACKUP_INTERVAL` 可设为不小于 1 分钟的 Go duration，`KARTWO_BACKUP_RETENTION` 默认保留最近 7 份、可设 1–365。自动备份先由既有导出内核完整生成临时 ZIP，再原子改名为 `data/backups/kartwo-backup-<UTC>.zip`；清理仅匹配这一前缀，绝不删除后台手工导出或其他文件。验收确认：恢复库连续启动两次、保留数设为 1 后，自动备份文件计数为 1。单测覆盖持久落盘、仅清理最旧自动备份、保留数和配置非法值拒绝；全量测试、vet、lint、govulncheck、gitleaks 均通过。
- **阶段**：**M5.7 全量导出 ZIP 恢复 ✅ 已人工验收合主干**（`PR #9` → `main`）。新增 `kartwo restore <export.zip>` CLI：只接受格式版本匹配的导出包，校验条目白名单、重复项、路径穿越、普通文件属性、数量与解压大小上限；先解压到同盘 0700 临时目录并执行 SQLite `PRAGMA integrity_check`，完成后才原子改名为目标数据目录。恢复目标必须不存在，拒绝覆盖任何既有店铺数据；恢复完成后以该目录启动服务，商家用原主口令登录。验收确认：导出包恢复至 `/data/m5restore`，日志显示源版本 `0.5.0-m5.6`、2 个媒体文件，目录含数据库且不含证书缓存；以恢复目录启动后可用原主口令进入后台，概览显示 4 个商品。单测覆盖导出→恢复的数据库与媒体完整性、既有目标拒绝与恶意 ZIP 路径拒绝；全量测试、vet、lint、govulncheck、gitleaks 均通过。
- **阶段**：**M5.6 全量数据导出 ZIP ✅ 已人工验收合主干**（`PR #7` → `main`）。Admin 新增“导出”入口和受鉴权保护的 `GET /admin/api/export` 下载；按导出瞬间创建 SQLite `VACUUM INTO` 一致性快照，连同 `data/media/` 打包为 ZIP。包内含 `manifest.json`（格式版本、UTC 创建时间、应用版本）、`shop.db`、`media/`；不包含 `data/certs/` 证书缓存。HTTP 传输结束即删除临时 ZIP 和数据库快照，文件权限为 0600。验收确认：浏览器成功下载 ZIP；解压可见 `manifest.json`、`shop.db`、`media/originals` 和 `media/derived`。单测已验证快照可重新打开、媒体内容存在、证书文件不被导出；全量测试、vet、lint、govulncheck、gitleaks 和前端构建均通过。
- **阶段**：**M5.5 本机健康诊断页 ✅ 已验收合主干**（`PR #6` → `main`）。Admin 新增“诊断”入口及只读 `GET /admin/api/diagnostics`；以实时探测而非模拟状态呈现数据库连接、未删除媒体资产数、原图/派生文件/合计占用，以及媒体目录所在磁盘的总量、可用量和已用量。数据库不可用返回 503；媒体统计失败返回 500；不支持读取磁盘容量的平台明确标为不可用，不伪造数字。验收确认：数据库连接正常；2 个媒体资产，原图 5.5 KB、派生文件 9.0 KB、合计 15 KB；磁盘总量 48 GB、可用 43 GB、已用 5.6 GB，数值关系正确。
- **阶段**：**M5.4 Shopify 旧链接 301 自动生成 ✅ 已验收合主干**（`PR #5` → `main`）。Shopify 导入成功时会在同一数据库事务保存 `Handle → 商品` 映射；访问旧路径 `/products/{handle}` 时，仅目标商品仍为上架状态才返回 `301 → /p/{slug}`，并保留查询参数。未知、草稿、归档和已删除商品一律返回 404，避免为不可公开商品暴露入口。验收确认：1 商品/1 变体导入成功、上架时旧链接跳转并保留查询参数、草稿时旧链接保持原路径并返回 404。迁移 `0014_shopify_redirect.sql` 可重入，新增映射、商品与导入任务同成同败。
  - **背景（第一轮诊断的三个实证发现）**：**P1** GitHub Releases 为空、无 release workflow、Makefile 无交叉编译目标、CI 不产 artifact → 北极星第一个动词「下载」当前无法按商家真实路径执行；**R1** 新建商品默认 `draft` 而店面只显示 `active`、中间零提示，且 `seed-demo` 硬编码 `active` 使历次自测**从结构上绕开**该坑；**R2** `:80` 无条件 301 到配置域名，DNS 未生效时店面与后台**同时失联且无自救路径**。
  - **本轮落地**：①`.github/workflows/release.yml`（push tag 触发、**现场构建前端再 embed**、四平台 + SHA256、流水线自检版本注入）+ `version` 子命令；②草稿态提示（编辑页随状态变化的 inline 说明 + 列表 chip 人话化与 `--warn` 着色，**不改默认值/后端/店面查询**）；③`httpsRedirect` 加 Host 判断 + 明文逃生路（`normalizeHost` 去端口/尾点/大小写，`ChallengeHandler` 加 app 参数，**不动 HostPolicy**）+ 11 个用例的回归单测。
  - **D8 已了结**：Admin session/CSRF 与店面购物车 cookie 均已改为按请求实际 TLS 判定，并经非回环 prod 明文真机实证通过。
  - **tag 纪律**：本轮只允许打**预发布 tag `v0.4.0-rc1`** 用于触发并验证 release 流程；**`v0.4.0-rc1` ≠ M4 收官**。正式 `v0.4.0` 须等北极星计时验收通过后才打。
- **阶段（前序）**：**M4.3（向导 SMTP 步骤 + 邮件队列 + 订单确认信）✅ 人工验收通过，已合主干**（分支 `feat/m4-mail` → `main`，**不打 tag**）。至此 **M4 功能项全部落地**，仅剩**北极星「30 分钟开店」计时验收**即可收官打 `v0.4.0`。
  - **验收四段全绿（2026-08-06 Derek 真机）**：**A 测试发信（D5）** Mailtrap 沙箱收到测试信，SMTP 全链路通；**B 订单确认信（核心）** Stripe 沙箱付款 → webhook 经 `stripe listen` 回本地 → 验签通过 → 订单 `019fd562` 转 **paid** → worker 20s 内发确认信到 Mailtrap，正文 `Your order 019fd562 is confirmed`、订单号/金额 `USD 99.00`/商品行（经典T恤 尺码M x1）全对、纯文本英文单模板（D9），**outbox→worker→SMTP 整条链路真机验实**；**C 未配置/未付不阻塞** 未配 SMTP 下单成功、`status=pending`、不卡，pending 未付不发确认信；**D 向导第 4 步** 进度条 N=4、「配置邮件」步、可跳过、字段集与文案诚实。
  - **实现要点**：`0011_email_outbox.sql`（`UNIQUE(order_id,kind)` 幂等锚点 + `ix_email_outbox_due` 检索索引）；`internal/mail` 包（config 设置键+env 旁路+KEK 绑定内存缓存 / smtp 发送 / outbox 入队与组信 / worker 轮询重试）；`internal/payment` 两处入队（`CapturePayPal` 同步 capture 路 + `markPaid` webhook 路），**n>0 才入队、与 pending→paid 同事务、入队失败仅记日志不阻断已付**；`internal/admin/smtp.go`（SMTP 设置读写/测试发信/向导状态与跳过，env 覆盖只读 409）；`main.go` 装配 worker 随优雅关停退出；前端 `SmtpSettings.vue` + `SmtpWizard.vue` + 向导进度 N=3→4。
  - **零新增依赖**：全部走 stdlib `net/smtp` + `crypto/tls`，`go.mod`/`go.sum`/前端 `package.json` 均未动（守「默认无外部依赖」底线）。
- **阶段（前序）**：**M4.2（向导完整化 + Admin UI 完善）✅ 整体收官**（M4.2.1 向导补完 + M4.2.2 概览 + M4.2.3a toast 迁移 + M4.2.3b 美化两片 全部验收合主干）。已拍板 **M4.2 先于 M4.3**、M4.2 切片。
  - **M4.2.1 向导补完（域名步骤 + 一气呵成外壳）✅ 人工验收通过，已合主干**（分支 `feat/m4-domain-wizard` → `main`，**不打 tag**）。验收覆盖：主线三步连贯（第 X/3 步进度条）、域名录入 + dev 文案诚实（本地开发模式明确「不会真的签发 HTTPS」）、非法输入后端拦截（`http://`/路径/`localhost`/空格 → 拒）、域名步「上一步」回收款步且**退不回已配市场**、留空跳过持久化、店面 HTTP 评估态可访问；env 只读态以 curl 自测为准（`source=env`/`readonly=true`/PUT→409）。
  - **M4.2.2 dashboard 概览 ✅ 人工验收通过，已合主干**（分支 `feat/m4-dashboard` → `main`，**不打 tag**）。验收覆盖：空态诚实（0/友好占位）+ 开店进度三卡（无商品/未配收款/未配域名，可点击跳转）+ 随配置消长 + 全齐「开店就绪」；库存告警零/低分类正确（可售=quantity−reserved，低库存 3≤5 归低库存，N=5）；种子订单 o1–o4 实测：今日 3 / US$100.00（**refunded o3 的 $30 已扣除**，D6 命门验实）、近7日 4 / US$120.00、待处理 3（D2，refunded 不计）；概览登录默认落点。**D1 时区**曾疑似 bug，真机复现证明边界正确（"全 0"实为连到遗留旧实例、非 m422），并补 `TestDashboardWindowBounds` 跨 UTC 日界确定性回归测试锁死。
  - **M4.2.3a 全站提示迁移到 toast ✅ 人工验收通过，已合主干**（分支 `feat/m4-toast-migration` → `main`，**不打 tag**）。判据=瞬时事件（操作成功/失败/动作级校验）→ toast；持续状态（页级加载失败/404/空态/静态说明）→ 保留 inline。迁 8 处动作反馈（ProductList 删除成功「商品已删除」+失败、PaymentSettings 保存、PaymentWizard 跳过失败、MarketSelect 选定、ProductEdit 生成校验/基本信息/传图/删图、OrderDetail 退款）；清理死掉的 `msg`/`err` 残留；**保留** 6 处页级 load 失败 inline（验实「订单不存在」为常驻红字非一闪 toast）、confirm 两处未动、toast 机制未改、后端零改动。
  - **本片范围边界（明确不碰，留后续片）**：SMTP 全部并入 **M4.3**（与发信机器端到端做）；slug 自动、上传进度 = 后续体验片。
  - **附带安全补丁（M4.2.3a 收尾时 govulncheck 新亮红灯）**：Go 工具链 **1.26.4→1.26.5** 修 **GO-2026-5856**（`crypto/tls` ECH 隐私泄露，真实可达）；独立 `chore(security)` commit；复扫 reachable 漏洞=0，门禁复绿。留档不处理 GO-2026-5932（x/crypto，无 fix、不可达不门禁）。详见 DECISIONS。
  - **附带安全补丁（M4.2.3b-① 收尾时 govulncheck 又亮红灯）**：依赖 `golang.org/x/text` **v0.38.0→v0.39.0** 修 **GO-2026-5970**（无效输入无限循环 DoS，真实可达 http.Serve→Unicode 规范化）；独立 `chore(security)` commit；复扫 reachable=0，门禁复绿。留档不处理 GO-2026-5942（x/net）、GO-2026-5932（x/crypto）——不可达不门禁。详见 DECISIONS。
  - **M4.2.3b-① 浅色 Stripe 风基调 ✅ 人工验收通过，已合主干**（分支 `feat/m4-ui-tone` → `main`，**不打 tag**）。建设计 token 层（色板/间距 4px 基/圆角/阴影/字阶）+ 全局元素改造（body/button/input/table/panel/卡片/字阶）；靛蓝 `#635bff` 主色、白卡细边弱阴影、保留 system-ui。**实现手法**：旧 token 名（`--panel/--line/--muted/--ok`）**别名映射**到新浅色板 → 全站组件自动吃新值不逐页翻改。破形最小微调仅 4 处 on-accent 文字 + 代表页 tile 阴影。
  - **M4.2.3b-② 细节 + confirm 统一弹窗 + 页级错误状态块 ✅ 人工验收通过，已合主干**（分支 `feat/m4-ui-details` → `main`，**不打 tag**）。confirm 统一模态（`confirm.js`+`ConfirmDialog.vue`，替换原生 confirm，Esc/遮罩=取消、破坏性确认红实心，语义不变）；页级错误状态块（`ErrorState.vue`，7 页，⚠️+说明+重试，常驻 inline 非 toast）；细节 token 化（退款徽章、OrderDetail 去重 `.danger`、订单/收款/市场页 scoped var 转新名、就绪态绿底）；别名永久保留为无害垫片；布局型内联样式保留现状（不镀金，新纪律入 CONVENTIONS §8.5）。**附带独立安全 bump：x/net v0.55→v0.56 修 GO-2026-5942（卫生，不可达）**。
- **M4.1（自动 HTTPS + 债1 闭环 + 债2）✅ 已合主干**（不打 tag）。验收实证：①autocert LE Staging 预跑→生产真证浏览器绿锁、HSTS 门控正确、HTTP 301 保留路径、domain_source=env；②债1 PayPal webhook 真实端到端验签闭环（真付款→paid、真退款→refunded，退款 webhook 带真实 event_id `WH-9ML49990950259907-…` 同步，金额整数分实证）；③债2 Stripe-Version 钉死。
- **债1 PayPal webhook 真实验签**：✅ **已了结（2026-07-06 真机验收）**——M3.3b-2 推迟项闭环，冒烟清单第 3 条已勾。
- **债2 Stripe-Version 钉死**：✅ **已了结（2026-07-06，选项 A：我方常量 `2026-06-24.dahlia` 不引 SDK）**。
- **M4.1 后一批小修/补全（均已 Derek 验收合主干、不打 tag）**：① CJK 竖排 bug（`214cd58`+`e45d43f`）；② 商品改价缺口补全 + 0 价必填口径（`c9c9453`+`b503c2d`+`6578a96`）；③ 轻量 toast 通知机制 + 视口居中，先接改价/新建提示（`b2f0d82`+`01efe0d`）。
- **🔴 `v0.4.0` 打 tag 的阻塞 gate（两项并列，缺一不可）**：
  1. **北极星「30 分钟开店」计时验收通过**：2026-08-20 Derek 决定跳过，当前状态为**未验收/未通过 gate**。
  2. **GitHub CI 修绿**。原始日志已取得，根因坐实：`golangci-lint-action@v6` 明确不支持 lint v2，须升 v7；`gitleaks-action@v2` 因组织仓库要求商业 license，在真正扫描前退出。当前修复片改为 action v7 + 固定版本开源 gitleaks CLI，并让本地 `make check` 纳入同一密钥扫描。
     - **PR #1 首轮又揭出两个真实 gate**：lint v2.5.0 官方预编译包由 Go 1.25 构建，拒绝分析目标 Go 1.26.5 → 改为用 CI 当前 Go 从源码安装，再由 action v7 运行；govulncheck 新报 7 个真实可达漏洞 → Go **1.26.5→1.26.6** 修 6 个标准库漏洞，`golang.org/x/image` **v0.43.0→v0.45.0** 修 WebP VP8L 过量内存分配。均不降级门禁、不加例外。
     - **PR #1 第二轮确认的工具元数据问题**：gitleaks v8.30.1 的 Go module path 是 `github.com/zricethezav/gitleaks/v8`，并非组织迁移后的路径；因此安装阶段冲突、扫描未运行。CI 与 Makefile 均已改为该声明路径；本地实跑与 **第三轮 PR CI** 均通过。
     - **第三轮 CI（run #17）四项全绿**：构建与测试（含 `go vet`、单元测试、静态二进制构建）、静态检查、漏洞与密钥扫描（`govulncheck` + `gitleaks`）、Admin 前端构建均通过。PR 仍为草稿，尚未合并到 `main`。
     - **⚠️ 取材方式（曾连续三轮卡在此处，故钉死）**：执行端读不到截图，匿名 API 取 job 日志返回 403「Must have admin rights」。取报错原文用 **`gh run view 31072574900 --log-failed`**，或网页展开红色步骤后**复制文字**。
- **下一步**：为退款成功路径补充审计和回归测试。M5.9 在具备可写 HTTPS WebDAV 目标后补验收。M4 的正式 `v0.4.0` 仍受既有发布 gate 约束。
- **最新 git tag**：`v0.3.0`（M3）。M4.1、M4.2 各片、M4.3 及其间小修均已合主干，按切片纪律不单独打 tag。**`v0.4.0-rc1` 为预发布 tag**（仅用于触发验证 release 流程，不代表 M4 收官）；正式 `v0.4.0` 待北极星计时验收通过后打。

## 里程碑总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 地基与骨架（含数据层选型落地、CI 安全门禁、生成各 .md） | ✅ 已验收通过（v0.0.0） |
| M1 | 核心数据模型 + Admin 基础 + 媒体上传 + StoragePolicy（切 5 片） | ✅ 已验收通过（v0.1.0） |
| M2 | 店面 + 购物车 + 下单（防超卖）+ SEO 基建（切 3 片） | ✅ 已验收通过（v0.2.0） |
| M3 | 支付路由 + Stripe/PayPal + 沙箱 + 退款 + 市场框架（切 3 片） | ✅ 已验收通过（v0.3.0） |
| M4 | 自动 HTTPS + 向导完整 + 30 分钟开店（北极星）**+ 承接：PayPal webhook 真实端到端验收（M3.3b-2 推迟项）** | 🟡 进行中（M4.1 ✅、**M4.2 整体 ✅**、**M4.3 ✅** 均已合主干；**功能项全部完成，仅剩北极星 30 分钟计时验收**，过后打 `v0.4.0`） |
| M5 | 数据导入(含301) + 诊断页 + 备份/导出/升级 | 🟡 进行中（M5.1 通用 CSV 导入地基 ✅；M5.2 Shopify CSV 适配 ✅；M5.3 图片本地化与 SSRF 防护 ✅；M5.4 Shopify 旧链接 301 ✅；M5.5 本机健康诊断 ✅；M5.6 全量数据导出 ZIP ✅；M5.7 ZIP 恢复 ✅；M5.8 本地自动备份 ✅ 已合主干；M5.9 WebDAV 异地备份待人工验收；M5.10 升级保护 ✅ 已合主干；M5.11 备份与升级快照诊断 ✅ 已合主干；M5.12 自动备份设置 ✅ 已合主干） |
| M6 | v1.1 硬化（审计/签名/i18n/法律模板/Woo导入/S3）+ 验收 | 🟡 进行中（M6.1 审计日志地基 ✅ 已验收合主干；M6.2 关键后台操作审计 ✅ 已验收合主干；M6.3 配置变更审计 ✅ 已验收合主干；退款审计进行中） |

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
  - [x] **M4.2.3b-① 浅色 Stripe 风基调（设计 token 层 + 全局元素改造）**（✅ 2026-07-22 人工验收通过，已合主干，不打 tag）：色板/间距/圆角/阴影/字阶 token；靛蓝 `#635bff` 主色、白卡细边弱阴影、保留 system-ui；旧 token 名别名映射全站自动生效；破形微调 4 处 on-accent + 代表页 tile 阴影
  - [x] **M4.2.3b-② 逐页细节打磨 + confirm 统一确认弹窗 + 页级错误状态块**（✅ 2026-07-28 人工验收通过，已合主干，不打 tag）：confirm 统一模态（promise 化，Esc/遮罩=取消、破坏性红实心，替换原生 confirm 语义不变）；`ErrorState.vue` 页级错误常驻块（7 页，非 toast）；细节 token 化（徽章/去重 .danger/scoped var/就绪态绿底）；别名永久保留；布局内联样式保留（不镀金）
- **M4.2 整体收官** ✅（4 切片全部验收合主干；tag 待 M4 整体收官统一打 `v0.4.0`）
- [x] **M4.3 向导 SMTP 步骤 + 邮件队列 + 订单确认信**（✅ 2026-08-06 人工验收通过，已合主干，不打 tag）：
  - **outbox 表**：`migrations/0011_email_outbox.sql` 纯 SQL 幂等；`UNIQUE(order_id,kind)` 作幂等锚点（`INSERT OR IGNORE` 依赖它，PayPal「同步 capture + webhook 备份」双触发只出一封）；`ix_email_outbox_due(status,next_attempt_at)` 供 worker 认领；状态机 `pending|sending|sent|failed|skipped`
  - **确认信触发（D10）**：落点=支付确认后的**两处** `MarkOrderPaidByPublicID`——`CapturePayPal`（同步 capture 路，本片为此新开事务）与 `markPaid`（Stripe/PayPal webhook 共用路）；**仅 n>0（真发生 pending→paid）才入队**、与状态变更**同事务原子**、入队失败仅 `slog.Error` **不阻断已付**
  - **不阻塞下单**：`order.Checkout` **零改动**——确认信只挂在「已付」之后，未付订单不入队、下单路径不碰 SMTP
  - **worker（D6/D8）**：20s ticker 轮询 + 启动即跑一轮；认领 `pending→sending` 是快速独立事务，**SMTP 网络 I/O 在事务/连接之外**（守 SQLite 单连接纪律）；失败指数退避 1m/5m/30m/2h/6h、**满 5 次标 failed 死信**；启动先 `ResetStaleSending` 恢复陈留 sending
  - **配置（D1/D2/D7-A）**：SMTP 密码 KEK 加密存、其余明文；`SMTP_*` env 覆盖旁路（env>加密库、覆盖非双写、worker 不依赖登录即可冷启动发信）；env 模式设置页只读、PUT 拒写 **409**；库来源缓存绑定 KEK 金库（登录解锁/登出销毁）；**真未配置→标 `skipped` 不重试**，**已配但金库锁定→全留 `pending`** 待登录后补发
  - **邮件内容（D9）**：纯文本、英文单模板、无营销；含订单号/总额（整数分→展示串）/逐行商品（含变体标签与行金额）
  - **UI**：向导第 4 步「配置邮件」（进度条 N=3→4、可跳过并持久化 `wizard.smtp_skipped`）+ 后台 SMTP 设置页 + **测试发信**按钮（D5）
  - **零新增依赖**：stdlib `net/smtp`+`crypto/tls`（none/STARTTLS 587/隐式 TLS 465），`go.mod` 未动
  - 单测：`internal/mail` 275 行（配置解析/env 覆盖/缓存生命周期/组信/worker 重试与死信/skipped 路径）+ `internal/admin` SMTP 端点测试（鉴权/CSRF/env 只读 409）
- [ ] **M4 北极星前置补丁**（🟡 待 Derek 验收，分支 `feat/m4-final-prep`，三项并为一片）：
  - **D1-A release 产物**：`.github/workflows/release.yml` —— 只在 push tag 触发；**严格串行 `npm ci && npm run build` → 校验 dist 存在 → `go build`**（不沿用 ci.yml 两个无 `needs:` job 的模式，杜绝 embed 陈旧前端）；`CGO_ENABLED=0 -trimpath -ldflags "-s -w -X main.Version=<tag>"`；四平台（linux/amd64 必需 + linux/arm64 + darwin/arm64 + windows/amd64）+ `SHA256SUMS.txt`；**流水线自检 `kartwo version` 输出 == tag**；含 `-` 的 tag 自动标 prerelease。新增 `version` 子命令。**Gitee 不做自动化**（本轮仅代码镜像）
  - **D2-B 草稿提示**（纯前端，**不改默认值/后端/店面查询**）：ProductEdit 状态下拉下方随状态变化的**常驻 inline 说明**（草稿=⚠️「不会出现在店面…请选『上架』」；上架=✅；归档=说明）——持续状态故 inline 不 toast；ProductList chip 人话化（`草稿 · 店面看不到`/`上架`/`归档`）+ 草稿态 `--warn/--warn-bg` 着色（新增 token 对，守 §8.5 无内联样式）
  - **D7-B 修 R2**：`normalizeHost`（`net.SplitHostPort` 去端口 + 去尾点 + 小写）；`httpsRedirect(domain, fallback)` —— Host **等于**配置域名才 301，其余（裸 IP / 其它域名 / 空 Host / IPv6 字面量）直接服应用作**明文逃生路**；`ChallengeHandler(m, domain, app)` 加 app 参数，ACME challenge 仍由 autocert 优先截获；**不动 HostPolicy**。单测 11 例（匹配/带端口/大小写/尾点/裸IP/IPv6/他域/空 Host/空域名/ACME 优先）
  - **任务 E 遗留诊断结论**：**「创建后不能改价」缺口确认已闭环**，PROGRESS 勾选状态与代码实况一致，无需处理（详见回报）
  - **任务 C 证伪结论（第二轮补充）**：「`CGO_ENABLED=0` 却动态链接」**经 Linux 原生 `file`/`readelf -d`/`readelf -l`/`ldd` 四方交叉验证——先前读数属实，不是误读**。根因链已锁定：`internal/media` → `gen2brain/webp` → `ebitengine/purego` 的 `//go:cgo_import_dynamic "libdl.so.2"`（逐依赖二分实证：hello world/sqlite/autocert 皆静态，webp/purego 皆动态）。影响：glibc 系正常、**musl(Alpine)/scratch 镜像跑不起来**。**已验证修复路 `-tags nodynamic`**（→ `statically linked`，全仓 13 包测试全绿，WebP 处理 380→423ms）。**本轮只诊断不修**，待排期
  - **任务 D（第二轮补充）**：release notes + README 下载段**逐平台标注验证状态**（linux/amd64 已验证；其余三平台未验证=仅编译通过），并标注 glibc 依赖与 musl 不支持
  - **D8-A cookie `Secure` 修复（第三轮补充，前提已坐实）**：Derek 非回环真机实证（`192.168.0.132:8080` + prod + Chrome）—— login 200 但两条 cookie 被整条丢弃、`/me` 401。落地 `secureFor(r)=r.TLS!=nil` 取代静态 `h.secure`，**session 与 csrf 两条一起改**（只修 session 会退化成「能登进后台但写操作全 403」，更隐蔽）；`setCookie`/`clearCookie` 均加 `r`（登出清除指令属性也须匹配）；`HttpOnly`/`SameSite=Lax` 不变。**修复实证**：prod 明文非回环跑通 建管理员 201 → 登录 200（两条 cookie 均无 Secure）→ `/me` 200 → 建商品 201。单测 4 分支 + 登出 + 属性无回归。附带 `InsecureNotice.vue` 明文访问常驻提示（http 且非回环才显示、inline 不 toast、零内联样式）
  - **第三处 cookie（店面购物车）已修**（第四轮，规划侧裁定「现在就修」）：`cart_http.go` 设置 + `checkout_http.go` 清除均改 `secureFor(r)`；`h.secure` 除此外已无用途 → 连同 `NewHTTP` 的 `secure` 参数移除。**全仓 grep 兜底确认设置 cookie 恰好 4 处、全部收敛，无第四处**。单测 4 分支 + 实证（明文非回环：加购 200 无 Secure → count=2 → 再加 count=3，同一辆车复用）
  - **推理校准（第四轮）**：DECISIONS 中 D8 登出一条原写「属性不匹配所以浏览器不认这条清除指令」——**结论对但机理错**。`Secure` 不属于 cookie 身份三元组（name/domain/path），删除不靠它匹配；真机理是**明文来源发出的带 Secure 的 Set-Cookie 被整条丢弃**，清除指令根本没被处理。已更正并留痕（照错误推理去推 `SameSite` 等属性会推歪）
  - **未做（材料未到，按纪律停手）**：**任务 B（CI 修复）** —— 要求「据实定位、不拿假设当结论」，而**两个红 job 的报错原文连续两轮均未随指令送达**，未动一行

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
- [x] **（M4.3 后）沙箱支付→订单已付→顾客收到订单确认信 可走** —— ✅ 2026-08-06 Derek 真机验收通过（Stripe 沙箱付款→webhook 验签→订单 `019fd562` 转 paid→worker 20s 内发信到 Mailtrap，正文订单号/金额/商品行全对）；同时验实**未配 SMTP 不阻塞下单**、**未付不发信**
- [x] **（M5.4 后）Shopify CSV 导入→图本地化→301 生成 可走** —— ✅ 2026-08-22 人工验收通过：上架商品旧路径 301 到 `/p/{slug}` 并保留查询参数；草稿商品旧路径保持 404。
- [x] **（M5.5 后）Admin → 诊断可读数据库、媒体占用与媒体目录磁盘状态** —— ✅ 2026-08-22 人工验收通过：数据库正常；2 个媒体资产，原图 5.5 KB、派生文件 9.0 KB、合计 15 KB；磁盘总量 48 GB、可用 43 GB、已用 5.6 GB，页面数值关系正确。
- [x] **（M5.6 后）Admin → 导出 ZIP 含 SQLite 快照和媒体文件，且不含证书缓存** —— ✅ 2026-08-22 人工验收通过：浏览器下载成功；解压可见 `manifest.json`、`shop.db`、`media/originals`、`media/derived`，符合导出边界。
- [x] **（M5.7 后）导出 ZIP → 新数据目录恢复 → 启动服务并以原主口令登录** —— ✅ 2026-08-23 人工验收通过：`m5-export.zip` 成功恢复到 `/data/m5restore`（源版本 `0.5.0-m5.6`、2 个媒体文件），`shop.db` 存在且未恢复 `certs/`；以该目录启动后原主口令可登录 Admin，概览显示 4 个商品。
- [x] **（M5.8 后）服务启动自动生成 ZIP，且只保留配置份数的自动备份** —— ✅ 2026-08-23 人工验收通过：恢复库连续启动两次，`KARTWO_BACKUP_RETENTION=1` 时 `kartwo-backup-*.zip` 计数为 1。
- [ ] **（M5.9 后）本地自动备份上传至 HTTPS WebDAV，远端出现同名 ZIP** —— 待具备可写 HTTPS WebDAV 目标后验收。
- [x] **（M5.10 后）已有数据目录升级前自动生成升级快照，新增迁移无损应用** —— ✅ 2026-08-23 人工验收通过：首次启动生成 `kartwo-upgrade-20260823T050548Z.zip` 并应用 1 条迁移；修复后的二次启动显示 `newly_applied=0`、本地自动备份完成且无 panic，升级快照仍存在。

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
