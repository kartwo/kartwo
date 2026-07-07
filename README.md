# Kartwo

极简到非技术商家也能自部署的跨境独立站电商后端。Go 单静态二进制 + 内嵌 SQLite，数据即文件夹，可跑 1C1G $5 VPS。

> 一套代码、一个内核：开源 ⊂ 自部署商业 ⊂ SaaS。本仓库 = 单租户内核（v1）。
> 权威文档：[ARCHITECTURE](ARCHITECTURE.md) · [ROADMAP](ROADMAP.md) · [PROGRESS](PROGRESS.md) · [DECISIONS](DECISIONS.md) · [CONVENTIONS](CONVENTIONS.md) · [LICENSES](LICENSES.md)

## 当前状态

已验收：**M0** 地基骨架 · **M1** 商品/变体/分类/库存 + Admin + 媒体上传 · **M2** SSR 店面 + 购物车 + 下单（防超卖）+ SEO 基建 · **M3** 支付路由 + Stripe/PayPal + 沙箱 + 退款。
进行中：**M4** 自动 HTTPS + 向导完整 + 30 分钟开店。进度以 [PROGRESS.md](PROGRESS.md) 为准。

## SEO 优势

独立站的自然流量命门在于**搜索引擎能不能顺畅地抓取、理解、索引你的商品页**。Kartwo 的店面把这层技术基建做扎实：

1. **服务端渲染（SSR）store­front** —— 店面用 Go `html/template` 在服务端直接吐出完整 HTML，爬虫无需执行 JavaScript 就能拿到全部内容。相比依赖前端 JS 渲染的 SPA / headless 电商，这是最根本的 SEO 优势：内容对搜索引擎**首屏即可见**。
2. **结构化数据 JSON-LD**（Product + AggregateOffer） —— 用 schema.org 标注商品与价格区间，帮搜索引擎准确理解页面是"什么商品、什么价"，从而有机会获得富媒体搜索结果（rich results）。
3. **规范化标签** —— `canonical`（收敛重复内容、避免自我竞争）、Open Graph（社交平台分享时的标题/图片预览）。
4. **sitemap.xml + robots.txt** —— 自动生成站点地图帮爬虫发现全部商品页；robots 放行公开页、屏蔽后台（`/admin/`）。
5. **WebP 响应式多尺寸图** —— 服务端生成更小体积的现代格式图片、按屏幕给合适尺寸，缩短首屏时间。页面速度是 Google 排名因素之一、WebP 是 Google 推荐的现代图片格式（加分项，非决定性）。

> **诚实边界**：以上是**技术 SEO 基建**——系统能控的部分做到位。搜索排名与流量最终还取决于内容质量、外链、行业竞争等**非技术因素**，不在系统的承诺范围内。Kartwo 保证的是"不让技术拖后腿"，不是"保证排名或流量"。
> 注：多语言 / hreflang 等国际化 SEO 属 v1.1，当前版本未实现。

## 部署与运行

分三层，从"本机试一试"到"对外卖货"到"进阶加固"。**桌面系统（macOS / Windows）用于本地评估试用，不是生产卖货环境**——生产请用 Linux 服务器。

### 第 1 层 · 本地评估 / 试用（macOS / Windows / Linux，跨平台）

在自己电脑上把系统跑起来、录商品、熟悉后台与店面，**纯本地、无公网 HTTPS**。

获取二进制：当前阶段从源码构建（需 Go 工具链）——`go build -o kartwo ./cmd/kartwo`。
> 预编译的三平台二进制将随首个正式 release 提供（M4 收官后）。

HTTP-only 评估态运行（不配域名，统一用高位端口 `:8080` 回避不同系统绑低端口的权限差异）：

```bash
# macOS / Linux
KARTWO_ENV=prod KARTWO_HTTP_ADDR=:8080 KARTWO_DATA_DIR=./data ./kartwo serve
```
```powershell
# Windows PowerShell
$env:KARTWO_ENV="prod"; $env:KARTWO_HTTP_ADDR=":8080"; $env:KARTWO_DATA_DIR="./data"; .\kartwo.exe serve
```

浏览器打开 `http://localhost:8080/`（店面）与 `http://localhost:8080/admin/`（后台）即可预览。

> 部署到远程 Linux 服务器时，先把本地构建好的二进制拷上去（`<IP>` 换成你服务器的公网 IP）：
> ```bash
> scp ~/bin/kartwo-linux-amd64 root@<IP>:~/kartwo
> ```

- 这是**受支持的 HTTP-only 评估态**：未配域名时不启 TLS、也**不发 HSTS**（避免把无证书的本地站锁死跳 HTTPS）。
- **仅供本地试用，无公网 HTTPS**；对外卖货请走第 2 层。

<!-- TODO(M4.1 段二后)：Derek 在 macOS 亲验 darwin 构建后回填校正本层实际命令与预期输出 -->

### 第 2 层 · 生产部署（Linux 服务器 / VPS）—— 待补全

> 🚧 **待 M4.1 段二真机验证通过后，据实提炼真实跑通的流程 + 真实北极星计时写入。** 本轮先占位，不写未经真机验证的具体步骤。

将覆盖的结构（北极星路径：下载运行 → 配好收款 → 发布首个商品 → 店面带 HTTPS 可访问）：

- 下载 Linux 二进制 → 在 VPS 运行（数据即文件夹，`KARTWO_DATA_DIR` 指向持久目录）。
- 授权绑定 `:80` / `:443`（`setcap 'cap_net_bind_service=+ep'`，免全程 root）。
- 开机向导填域名 → **内嵌 autocert 自动签发 / 续期 HTTPS**（无需手动折腾证书）。
- 环境变量：`KARTWO_ENV=prod`、`KARTWO_DOMAIN`、`KARTWO_HTTP_ADDR` / `KARTWO_HTTPS_ADDR`、`KARTWO_ACME_DIRECTORY`（可指 Let's Encrypt Staging 预跑）。
- 最低配置：1C1G VPS；需在**云厂商安全组**放行 80 / 443 入站。

### 第 3 层 · 进阶（可选）

- **Cloudflare 橙云 + Origin Certificate**：把域名开橙云代理，隐藏源站 IP、叠加 CDN 与防护；此时对外 TLS 由 Cloudflare 边缘接管，源站换用 Cloudflare Origin Certificate、内嵌 autocert 让位。（结构说明，具体步骤后续补。）

## 开发与门禁

```bash
go build -o kartwo ./cmd/kartwo   # 单静态二进制
KARTWO_ADDR=:8080 ./kartwo        # dev 默认监听 :8080，数据写入 ./data
curl http://localhost:8080/health # {"status":"ok",...}

make check   # 合主干前本地门禁：vet + test + build + lint + govulncheck
```

需安装 `sqlc` `golangci-lint` `govulncheck` `gitleaks`（版本见 [`.github/workflows/ci.yml`](.github/workflows/ci.yml)）。

## 许可

内核 MIT（见 [LICENSES.md](LICENSES.md) 登记的第三方资源许可）。
