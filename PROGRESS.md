# Kartwo — 项目进度（PROGRESS）

> 项目进度的**单一事实来源**。Claude Code 每轮收尾必须更新此文件。
> 进度以本文件 + git tag 为准，不依赖对话记忆。
> 作者：仗键天涯(daxing) ｜ 3442535897@qq.com
> 最后更新：2026-06-17（M0 验收通过，已合主干打 v0.0.0）

---

## 当前状态
- **阶段**：**M1.4（媒体上传 + StoragePolicy）已完成，待 Derek 人工验收**（分支 `feat/m1-data-model`）。
- **下一步**：M1.4 验收通过 → M1.5（Admin SPA，届时拍板 Vue3/React）。M1 各片累积于分支，全 M1 完成后合主干打 `v0.1.0`。
- **最新 git tag**：`v0.0.0`（M0）

## 里程碑总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 地基与骨架（含数据层选型落地、CI 安全门禁、生成各 .md） | ✅ 已验收通过（v0.0.0） |
| M1 | 核心数据模型 + Admin 基础 + 媒体上传 + StoragePolicy（切 5 片） | 🟡 进行中（M1.1–1.4 已做，1.4 待验收） |
| M2 | 店面 + 购物车 + 下单（防超卖）+ SEO 基建 | ⬜ 未开始 |
| M3 | 支付路由 + Stripe/PayPal + 沙箱 + 退款 | ⬜ 未开始 |
| M4 | 自动 HTTPS + 向导完整 + 30 分钟开店（北极星） | ⬜ 未开始 |
| M5 | 数据导入(含301) + 诊断页 + 备份/导出/升级 | ⬜ 未开始 |
| M6 | v1.1 硬化（审计/签名/i18n/法律模板/Woo导入/S3）+ 验收 | ⬜ 未开始 |

> 状态图例：⬜ 未开始 ｜ 🟡 进行中 ｜ ✅ 已验收通过

## 当前里程碑明细（M1 · 切 5 片）
- [x] **M1.1 数据模型与迁移**（✅ 已验收）：通用双轴 option×option schema、纯 SQL 迁移、sqlc 数据层、seed-demo 装 6 变体并打印矩阵、单测
- [x] **M1.2 Admin 鉴权 + 向导骨架**（✅ 已验收）：argon2id 口令、主口令派生 KEK(内存金库)、初始化幂等、会话+CSRF、登录限流、向导 API；单测+HTTP 测
- [x] **M1.3 Admin 商品 CRUD API**（✅ 已验收）：商品建/列/取/改/软删、变体校验、改库存、分类增列；鉴权+对象级+CSRF；单测+HTTP 测+实测
- [x] **M1.4 媒体上传 + StoragePolicy**（待验收）：multipart 上传、magic-bytes、去 EXIF、多尺寸 WebP(gen2brain)、内容哈希、本地后端、StoragePolicy+磁盘护栏、单商品张数护栏、孤儿清理、/media 托管；单测+HTTP 测+实测
- [ ] M1.5 Admin SPA（登录 + 商品管理；SPA 框架届时拍板）

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
- [ ] 主攻市场（美/欧/中东/东南亚）—— 阻塞 M3
- [ ] storefront v1 形态（暂定内嵌主题）—— 待确认

## 回归冒烟清单（每次合主干前 Derek 重跑，随功能增加）
- [ ] （M2 后）开店→浏览→加购→下单 主干可走
- [ ] （M3 后）沙箱支付→订单已付→退款 可走
- [ ] （M5 后）Shopify CSV 导入→图本地化→301 生成 可走

---

## 更新约定（Claude Code）
每轮收尾：① 更新里程碑状态与子任务勾选；② 更新"当前状态/下一步/最新 git tag"；③ 新决策同时写入 `DECISIONS.md`；④ 在回报中说明本文件改了什么。
