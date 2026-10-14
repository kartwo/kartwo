// 博客文章链接生成 / Blog Post Slug Generation
// 功能：将英文标题转换为安全链接，并避开现有文章链接
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-14 09:45:42

export function slugFromBlogTitle(title) {
  return title.toLowerCase().normalize('NFKD').replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 100).replace(/-+$/g, '')
}

export function uniqueBlogSlug(title, usedSlugs) {
  const base = slugFromBlogTitle(title)
  if (!base) return ''
  const used = new Set(usedSlugs)
  if (!used.has(base)) return base
  for (let number = 2; ; number++) {
    const suffix = `-${number}`
    const prefix = base.slice(0, 100 - suffix.length).replace(/-+$/g, '')
    const candidate = prefix + suffix
    if (!used.has(candidate)) return candidate
  }
}
