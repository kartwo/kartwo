// Admin SPA 嵌入 / Admin SPA Embedding
// 功能：将 React + TypeScript + Vite + Ant Design 后台构建资源嵌入二进制托管
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package admin

import "embed"

// FS 嵌入 dist 目录下的唯一正式 Admin SPA 静态资源。
//
//go:embed dist
var FS embed.FS
