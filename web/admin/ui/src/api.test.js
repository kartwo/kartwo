// 博客 API 载荷测试 / Blog API Payload Tests
// 功能：防止富文本正文在创建或更新文章时被客户端过滤掉
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 22:47:00

import assert from 'node:assert/strict'
import test from 'node:test'
import { api } from './api.js'

test('创建与更新文章都提交富文本正文', async () => {
  const originalDocument = globalThis.document
  const originalFetch = globalThis.fetch
  const requests = []
  globalThis.document = { cookie: '' }
  globalThis.fetch = async (url, options) => {
    requests.push({ url, method: options.method, body: JSON.parse(options.body) })
    return { ok: true, text: async () => '{}' }
  }
  try {
    const post = { title: '测试', slug: 'test', body_markdown: '', body_html: '<p><strong>正文</strong></p>', status: 'draft' }
    await api.createBlogPost(post)
    await api.updateBlogPost('post-id', post)
    assert.equal(requests.length, 2)
    assert.deepEqual(requests.map((request) => request.method), ['POST', 'PUT'])
    for (const request of requests) {
      assert.equal(request.body.body_html, post.body_html)
      assert.equal(request.body.body_markdown, '')
    }
  } finally {
    globalThis.document = originalDocument
    globalThis.fetch = originalFetch
  }
})
