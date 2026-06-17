# Kartwo — 资源许可登记（LICENSES）

> 所有第三方字体、图标、媒体等资源在引入时必须登记于此，记录名称/用途/来源/许可证/链接。
> 规则：仅允许**非商业开放许可**——字体 OFL/Apache-2.0；图标 MIT/Apache/ISC/CC0。
> 禁止：商用授权 / 付费 / 许可不清 / 来源不明。引入前确认许可证，引入后更新本表。
> 作者：仗键天涯(daxing) ｜ 3442535897@qq.com

---

## 字体

| 名称 | 用途 | 许可证 | 来源 | 状态 |
|---|---|---|---|---|
| Inter | UI 默认西文字体 | OFL-1.1 | https://github.com/rsms/inter | 计划 |
| Noto Sans | 多语言（含 CJK） | OFL-1.1 | https://github.com/notofonts | 计划 |
| Noto Sans Arabic | 阿拉伯语 RTL（若做中东市场） | OFL-1.1 | https://github.com/notofonts | 备选 |

## 图标

| 名称 | 用途 | 许可证 | 来源 | 状态 |
|---|---|---|---|---|
| Lucide | Admin/Storefront 图标 | ISC | https://github.com/lucide-icons/lucide | 计划 |
| Tabler Icons | 备选图标集 | MIT | https://github.com/tabler/tabler-icons | 备选 |
| Heroicons | 备选图标集 | MIT | https://github.com/tailwindlabs/heroicons | 备选 |

## 代码依赖（关键第三方库）

| 名称 | 用途 | 许可证 | 状态 |
|---|---|---|---|
| modernc.org/sqlite | 纯 Go SQLite 驱动（默认数据库） | BSD-3-Clause | 已引入 |
| github.com/google/uuid | 外部 ID（UUIDv7）生成 | BSD-3-Clause | 已引入 |
| golang.org/x/crypto/argon2 | 主口令哈希与 KEK 派生（argon2id） | BSD-3-Clause | 已引入 |
| 工具：sqlc | 由纯 SQL 生成类型安全数据层代码（构建期，不进二进制） | MIT | 已引入 |
| 工具：golangci-lint | 静态检查（CI 门禁，不进二进制） | GPL-3.0（仅作为外部 CLI 调用，不链接进产物） | 已引入 |
| 工具：govulncheck | 依赖漏洞扫描（CI 门禁，不进二进制） | BSD-3-Clause | 已引入 |
| 工具：gitleaks | 密钥泄漏扫描（CI 门禁，不进二进制） | MIT | 已引入 |

> 注：上述 sqlc/golangci-lint/govulncheck/gitleaks 均为开发/CI 期外部工具，不被编译链接进发布二进制，故其许可不影响产物分发。modernc.org/sqlite 的间接依赖（go-humanize/uuid 等）均为 BSD/MIT 系宽松许可，随 `go.mod` 管理。

## 其他媒体（示例图/demo 数据图等）

| 名称 | 用途 | 许可证 | 来源 | 状态 |
|---|---|---|---|---|
| （demo 数据用图须为 CC0 或自有） | | | | |

---

> 状态：计划 / 已引入 / 备选 / 已移除。每次引入或更换资源都要更新本表，并在 commit 中说明。
