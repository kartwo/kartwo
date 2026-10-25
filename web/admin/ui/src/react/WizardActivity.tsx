// 向导操作状态 / Wizard Activity
// 功能：配置加载和保存期间禁止向导提前切页
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:21:49

import { createContext, useContext, useEffect } from 'react'
export const WizardActivity = createContext<(busy: boolean) => void>(() => {})
export function useWizardActivity(busy: boolean): void {
  const update = useContext(WizardActivity)
  useEffect(() => { update(busy); return () => update(false) }, [busy, update])
}
