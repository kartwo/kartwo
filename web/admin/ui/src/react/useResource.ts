// 页面请求生命周期 / Page Request Lifecycle
// 功能：取消离开的读取、阻止重复操作，过期请求不覆盖当前会话
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:21:49

import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, errorMessage } from './transport'
import { useSession } from './SessionProvider'

export function useResource<T>(load: (signal: AbortSignal) => Promise<T>) {
  const { controller } = useSession()
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const pending = useRef<AbortController | null>(null)
  const refresh = useCallback(async () => {
    pending.current?.abort()
    const current = new AbortController()
    pending.current = current
    const identity = controller.getSnapshot().session
    setLoading(true); setError(''); setData(null)
    try {
      const next = await load(current.signal)
      if (!current.signal.aborted && identity === controller.getSnapshot().session) setData(next)
    } catch (failure) {
      if (current.signal.aborted || identity !== controller.getSnapshot().session) return
      if (failure instanceof APIError && failure.status === 401) controller.unauthorized()
      else setError(errorMessage(failure))
    } finally { if (!current.signal.aborted) setLoading(false) }
  }, [load, controller])
  useEffect(() => { void refresh(); return () => pending.current?.abort() }, [refresh])
  return { data, setData, loading, error, refresh }
}

export function useTask() {
  const { controller } = useSession()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const running = useRef(false)
  const mounted = useRef(false)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  async function run<T>(operation: () => Promise<T>, success?: (value: T) => void): Promise<boolean> {
    if (running.current) return false
    const identity = controller.getSnapshot().session
    if (!identity) return false
    running.current = true
    setBusy(true); setError('')
    try {
      const value = await operation()
      if (!mounted.current || identity !== controller.getSnapshot().session) return false
      success?.(value)
      return true
    } catch (failure) {
      if (!mounted.current || identity !== controller.getSnapshot().session) return false
      if (failure instanceof APIError && failure.status === 401) controller.unauthorized()
      else setError(errorMessage(failure))
      return false
    } finally {
      running.current = false
      if (mounted.current) setBusy(false)
    }
  }
  return { busy, error, run }
}
