// 博客文章链接测试 / Blog Post Slug Tests
// 功能：验证标题转换、重名递增与长度约束
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-14 09:45:42

import assert from 'node:assert/strict'
import test from 'node:test'
import { slugFromBlogTitle, uniqueBlogSlug } from './blogSlug.js'

test('英文标题生成可用文章链接', () => {
  assert.equal(slugFromBlogTitle('Ronaldo scores after match delayed by missile alert'), 'ronaldo-scores-after-match-delayed-by-missile-alert')
  assert.equal(slugFromBlogTitle('Café & Running!'), 'cafe-running')
  assert.equal(slugFromBlogTitle('中文标题'), '')
})

test('同名文章自动递增，结果保持在 100 字符内', () => {
  assert.equal(uniqueBlogSlug('Running News', ['running-news', 'running-news-2']), 'running-news-3')
  const title = 'A'.repeat(120)
  const first = slugFromBlogTitle(title)
  const second = uniqueBlogSlug(title, [first])
  assert.equal(first.length, 100)
  assert.equal(second.length, 100)
  assert.match(second, /^[a-z0-9]+(?:-[a-z0-9]+)*$/)
  assert.ok(second.endsWith('-2'))
})
