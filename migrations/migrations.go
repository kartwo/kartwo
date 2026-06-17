// 迁移文件嵌入 / Embedded Migrations
// 功能：将 migrations/*.sql 以 embed.FS 暴露给迁移执行器
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package migrations

import "embed"

// FS 嵌入本目录下全部 .sql 迁移文件，供 internal/migrate 在启动时应用。
//
//go:embed *.sql
var FS embed.FS
