// 会话上下文 / Session Context
// 功能：将可测试的会话生命周期提供给 React 页面
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:11:05

import { createContext, useContext, useEffect, useState, useSyncExternalStore, type ReactNode } from 'react'
import { SessionController, type SessionState } from './session'
const Context = createContext<{ state: SessionState; controller: SessionController } | null>(null)
export function SessionProvider({ children, controller: supplied }: { children: ReactNode; controller?: SessionController }) {
  const [controller] = useState(() => supplied ?? new SessionController())
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => { void controller.refresh(); return controller.dispose }, [controller])
  return <Context.Provider value={{ state, controller }}>{children}</Context.Provider>
}
export function useSession() {
  const context = useContext(Context)
  if (!context) throw new Error('页面需要 SessionProvider')
  return context
}
