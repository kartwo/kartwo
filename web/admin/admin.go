// Admin SPA 嵌入 / Admin SPA Embedding
// 功能：将构建好的 Admin 前端静态资源嵌入二进制托管（M0 为占位页）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package admin

import "embed"

// FS 嵌入 dist 目录下的 Admin SPA 静态资源；M1 起替换为真实构建产物。
//
//go:embed dist
var FS embed.FS
