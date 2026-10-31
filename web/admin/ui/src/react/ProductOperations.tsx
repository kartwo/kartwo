// 商品共享操作 / Shared Product Operations
// 功能：商品字段、翻译、变体与媒体共用写入锁及会话失效处理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { createContext, useContext } from 'react'
import { useTask } from './useResource'
export const ProductOperations = createContext<ReturnType<typeof useTask> | null>(null)
export function useProductOperations() { const value = useContext(ProductOperations); if (!value) throw new Error('商品操作上下文未提供'); return value }
