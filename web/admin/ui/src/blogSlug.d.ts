// 博客链接类型 / Blog Slug Types
// 功能：让 React 与原 Node 回归测试共享唯一的链接生成逻辑
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32
export function slugFromBlogTitle(title: string): string
export function uniqueBlogSlug(title: string, usedSlugs: string[]): string
