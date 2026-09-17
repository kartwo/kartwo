# Kartwo

极简到非技术商家也能自部署的跨境独立站电商系统。一个 Go 可执行文件内嵌 Admin 与英文店面，默认使用 SQLite，数据即文件夹，可运行在 1C1G VPS。

> 一套代码、一个内核：开源 ⊂ 自部署商业 ⊂ SaaS。本仓库是单租户内核。

## 当前版本

`v0.6.0-beta.1` 是首个公开开源测试版，可完整完成开店、上架、收款、发货与内容运营，但仍是 **beta**，建议先在测试店或小流量店铺验证并保持自动备份。

- Linux x86_64：发布流水线在干净数据目录实跑迁移、20 件演示商品、店面、后台与健康检查。
- Linux ARM64、macOS Apple Silicon、Windows x86_64：编译产出，尚未逐平台人工实跑。
- Linux 二进制依赖 glibc；Alpine/musl 暂不支持。

## 在线演示

- 演示店面：[https://www.kartwo.com/](https://www.kartwo.com/)
- 演示后台：[https://www.kartwo.com/admin/](https://www.kartwo.com/admin/)

演示后台无需账号或密码，打开后点击“进入公开演示”即可体验。每个访客会获得一段相互隔离的 45 分钟会话，可以浏览完整后台，并新建最多 3 个临时草稿商品；示例商品和关键配置受到保护，收款、邮件、备份及顾客资料等敏感信息会被隐藏，修改关键配置、订单操作、导入和导出会由服务端拒绝。退出、手动重置或会话过期后，临时商品及其图片会被自动清理。

公开演示仅用于产品体验，请勿输入真实顾客资料、支付密钥或其他敏感信息。

## 已具备能力

- 商品、双轴变体、库存、分类、图片 WebP 处理与首页精选
- 英文 SSR 店面、搜索、购物车、结账、防超卖与 SEO 基建
- Stripe / PayPal 收款、退款、订单邮件、后台订单与 CSV 导出
- 按洲选择配送国家、默认/特殊地区运费、发货与物流追踪
- 店铺名称与 Logo、内容页、7 个可编辑页脚政策页面
- Shopify CSV 导入与旧链接 301、本机诊断、审计、全量导出/恢复、自动备份
- 内嵌自动 HTTPS、初始化向导、单二进制升级保护
- 显式命令装入 5 个分类 × 4 件商品及可选原创封面，不会在启动或升级时污染真实店铺

版本变更和已知边界见 [CHANGELOG.md](CHANGELOG.md)。

## 下载与快速体验

从 [GitHub Releases](https://github.com/kartwo/kartwo/releases) 下载 `v0.6.0-beta.1` 对应平台文件和 `SHA256SUMS.txt`。

| 平台 | 文件 | 状态 |
|---|---|---|
| Linux x86_64 VPS | `kartwo-linux-amd64` | 已验证 |
| Linux ARM64 | `kartwo-linux-arm64` | 仅编译 |
| macOS Apple Silicon | `kartwo-darwin-arm64` | 仅编译 |
| Windows x86_64 | `kartwo-windows-amd64.exe` | 仅编译 |

Linux/macOS：

```bash
chmod 755 kartwo-linux-amd64
./kartwo-linux-amd64 version
KARTWO_ENV=prod KARTWO_HTTP_ADDR=:8080 KARTWO_DATA_DIR=./data ./kartwo-linux-amd64 serve
```

Windows PowerShell：

```powershell
.\kartwo-windows-amd64.exe version
$env:KARTWO_ENV="prod"; $env:KARTWO_HTTP_ADDR=":8080"; $env:KARTWO_DATA_DIR="./data"; .\kartwo-windows-amd64.exe serve
```

浏览器打开 `http://localhost:8080/` 和 `http://localhost:8080/admin/`。首次进入后台会创建管理员；数据持续保存在 `./data`，升级时不要删除该目录。

校验下载完整性：

```bash
sha256sum -c SHA256SUMS.txt --ignore-missing
```

### 可选：装入跑步演示店铺

同一 Release 的 `kartwo-running-demo-images.zip` 包含 20 张原创 WebP 封面。将压缩包与程序放在同一工作目录，先解压再显式装入：

```bash
unzip kartwo-running-demo-images.zip
KARTWO_ENV=prod KARTWO_DATA_DIR=./data ./kartwo-linux-amd64 seed-running-demo
```

### 可选：启用公开后台演示

公开演示必须使用独立实例和独立数据目录，禁止连接真实经营数据库。全新实例应先在演示模式关闭时创建真正管理员，再装入演示数据；没有管理员时程序会拒绝启动公开演示。

在 systemd 的 `[Service]` 中加入以下配置，即可提供无需密码、45 分钟、每会话最多 3 个临时草稿商品、每件最多 1 张 2MB 图片的隔离体验：

```ini
Environment=KARTWO_DEMO_MODE=true
Environment=KARTWO_DEMO_SESSION_TTL=45m
Environment=KARTWO_DEMO_MAX_PRODUCTS=3
Environment=KARTWO_DEMO_MAX_IMAGE_BYTES=2097152
Environment=KARTWO_DEMO_CLEANUP_INTERVAL=5m
```

演示身份不能修改基线商品、分类、内容、订单或关键配置。临时商品不会进入店面，并会在退出、重置或过期后连同图片清理。普通自部署默认关闭此模式，原有行为不变。

该命令只补缺，不覆盖已有商品、分类、政策正文或商品图片；重复运行安全。若不下载图片包，仍会生成 20 件商品，只是没有封面。

## Linux 生产部署

生产模式可直接监听 `:80` 和 `:443`，内嵌 Let's Encrypt 自动签发与续期证书，不强制依赖 Nginx 或 Certbot。完整步骤见[生产部署手册](docs/production-deployment.md)。

- 使用持久目录，例如 `/data/kartwo`，并定期验证备份可恢复。
- 放行 TCP 80/443；`KARTWO_DOMAIN` 只填域名，不含协议、路径或端口。
- 建议交给 systemd 守护；升级前先停止服务并备份完整数据目录。
- 收款先用 Stripe/PayPal 沙箱完成一笔付款与退款，再切正式密钥。

## SEO 基建

店面直接输出完整 HTML，并提供 Product/AggregateOffer JSON-LD、canonical、Open Graph、sitemap.xml、robots.txt 与响应式 WebP。它解决技术可抓取性，不承诺搜索排名；多语言与 hreflang 尚未实现。

## 从源码开发

要求 Go 与 Node 版本见 [CI 配置](.github/workflows/ci.yml)。

```bash
make gen
cd web/admin/ui && npm ci && npm run build && cd ../../..
go build -o kartwo ./cmd/kartwo
make check
```

`make check` 包含 vet、race test、build、lint、govulncheck 与 gitleaks。发布工作流会重新构建 Admin 后再嵌入二进制，避免提交产物陈旧。

## 许可

Kartwo 内核采用 [MIT License](LICENSE)。第三方依赖与原创媒体来源见 [LICENSES.md](LICENSES.md)。
