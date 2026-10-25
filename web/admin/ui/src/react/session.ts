// 会话生命周期 / Session Lifecycle
// 功能：防止过时请求恢复已退出会话，串行登录并明确区分认证失败与网络错误
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:11:05

import { APIError, errorMessage } from './transport'
import { sessionAPI, type SessionAPI } from './session-api'
import type { Session, WizardStep } from './contracts'

export interface SessionState {
  phase: 'loading' | 'setup' | 'login' | 'ready' | 'error'
  session: Session | null
  demoAvailable: boolean
  wizard: WizardStep | null
  busy: boolean
  error: string
}
const initialState: SessionState = { phase: 'loading', session: null, demoAvailable: false, wizard: null, busy: false, error: '' }
const wizardSteps: WizardStep[] = ['payment', 'domain', 'smtp']

export class SessionController {
  private state: SessionState = { ...initialState }
  private listeners = new Set<() => void>()
  private revision = 0
  private pending?: AbortController
  private working = false
  constructor(readonly api: SessionAPI = sessionAPI) {}
  getSnapshot = (): SessionState => this.state
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => { this.listeners.delete(listener) }
  }
  private publish(change: Partial<SessionState>): void {
    this.state = { ...this.state, ...change }
    this.listeners.forEach(listener => listener())
  }
  dispose = (): void => { this.revision++; this.pending?.abort() }
  unauthorized = (): void => {
    this.dispose()
    this.publish({ phase: 'login', session: null, wizard: null, busy: false, error: '' })
  }
  private async nextWizard(start: number, signal?: AbortSignal): Promise<WizardStep | null> {
    for (const step of wizardSteps.slice(start)) {
      if ((await this.api.wizard(step, signal)).needed) return step
    }
    return null
  }
  refresh = async (): Promise<void> => {
    const revision = ++this.revision
    this.pending?.abort()
    this.pending = new AbortController()
    const { signal } = this.pending
    this.publish({ phase: 'loading', session: null, wizard: null, error: '' })
    let demoAvailable = this.state.demoAvailable
    try {
      const status = await this.api.status(signal)
      demoAvailable = status.demo_mode
      if (!status.initialized) {
        if (revision === this.revision) this.publish({ phase: 'setup', demoAvailable })
        return
      }
      const session = await this.api.me(signal)
      const wizard = session.role === 'owner' ? await this.nextWizard(0, signal) : null
      if (revision === this.revision) this.publish({ phase: 'ready', session, wizard, demoAvailable })
    } catch (error) {
      if (revision !== this.revision || signal.aborted) return
      if (error instanceof APIError && error.status === 401) {
        this.publish({ phase: 'login', session: null, wizard: null, demoAvailable })
      } else {
        this.publish({ phase: 'error', session: null, wizard: null, demoAvailable, error: errorMessage(error) })
      }
    }
  }
  private async action(run: () => Promise<void>): Promise<void> {
    if (this.working) return
    this.working = true
    this.publish({ busy: true, error: '' })
    try { await run() }
    catch (error) { this.publish({ error: errorMessage(error) }) }
    finally { this.working = false; this.publish({ busy: false }) }
  }
  authenticate = async (mode: 'login' | 'setup' | 'demo', username = '', password = ''): Promise<void> => {
    await this.action(async () => {
      const revision = this.revision
      let initialized = false
      try {
        if (mode === 'setup') { await this.api.setup(username, password); initialized = true }
        if (mode === 'demo') await this.api.demo()
        else await this.api.login(username, password)
      } catch (error) {
        // Initialization already committed: retry login, never offer destructive reinitialization.
        if (initialized && revision === this.revision) this.publish({ phase: 'login', session: null, wizard: null })
        throw error
      }
      if (revision === this.revision) await this.refresh()
    })
  }
  logout = async (): Promise<void> => {
    await this.action(async () => {
      // Cancel bootstrap reads immediately, but show logout failure instead of claiming success.
      this.dispose()
      try { await this.api.logout() }
      catch (error) {
        if (!(error instanceof APIError && error.status === 401)) throw error
      }
      this.unauthorized()
    })
  }
  advanceWizard = async (skip: boolean): Promise<void> => {
    const { session, wizard } = this.state
    if (session?.role !== 'owner' || !wizard) return
    await this.action(async () => {
      const revision = this.revision
      try {
        if (skip) await this.api.skipWizard(wizard)
        const next = await this.nextWizard(wizardSteps.indexOf(wizard) + 1)
        if (revision === this.revision) this.publish({ wizard: next })
      } catch (error) {
        if (revision !== this.revision) return
        if (error instanceof APIError && error.status === 401) this.unauthorized()
        else throw error
      }
    })
  }
  previousWizard = (): void => {
    if (this.state.busy || this.state.session?.role !== 'owner' || !this.state.wizard) return
    const previous = wizardSteps[wizardSteps.indexOf(this.state.wizard) - 1]
    if (previous) this.publish({ wizard: previous, error: '' })
  }
}
