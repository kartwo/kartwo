# Kartwo

极简到非技术商家也能自部署的跨境独立站电商后端。Go 单静态二进制 + 内嵌 SQLite，数据即文件夹，可跑 1C1G $5 VPS。

> 一套代码、一个内核：开源 ⊂ 自部署商业 ⊂ SaaS。本仓库 = 单租户内核（v1）。
> 权威文档：[ARCHITECTURE](ARCHITECTURE.md) · [ROADMAP](ROADMAP.md) · [PROGRESS](PROGRESS.md) · [DECISIONS](DECISIONS.md) · [CONVENTIONS](CONVENTIONS.md) · [LICENSES](LICENSES.md)

## 当前状态

**M0 · 地基与骨架**：可编译的单二进制、`/health` 健康检查、纯 SQL 幂等迁移、安全响应头、内嵌 Admin 占位页、CI 安全门禁。业务功能见 ROADMAP（M1 起）。

## 快速开始（开发）

```bash
go build -o kartwo ./cmd/kartwo   # 单静态二进制
./kartwo                          # 默认监听 :8080，数据写入 ./data
curl http://localhost:8080/health # {"status":"ok",...}
```

可配环境变量：`KARTWO_ADDR`（默认 `:8080`）、`KARTWO_DATA_DIR`（默认 `./data`）、`KARTWO_ENV`（`dev`/`prod`）。

## 本地门禁（合主干前）

```bash
make check   # vet + test + build + lint + govulncheck
```

需安装 `sqlc` `golangci-lint` `govulncheck` `gitleaks`（版本见 `.github/workflows/ci.yml`）。

## 许可

内核 MIT（见 [LICENSES.md](LICENSES.md) 登记的第三方资源许可）。
