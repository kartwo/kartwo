// 后台主题 / Admin Theme
// 功能：由统一标准色板配置 Ant Design 默认算法、组件与深色导航
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:07

import { useState, type ReactNode } from 'react'
import { App, ConfigProvider, type ThemeConfig } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import 'antd/dist/reset.css'
import '../tokens.css'
import './styles.css'
export function createAdminTheme(): ThemeConfig {
  const styles = getComputedStyle(document.documentElement)
  const token = (name: string) => styles.getPropertyValue(`--${name}`).trim() || undefined
  return {
    token: {
      colorPrimary: token('accent'), colorLink: token('accent'), colorError: token('danger'),
      colorSuccess: token('success'), colorWarning: token('warn'), colorText: token('text'),
      colorTextSecondary: token('text-muted'), colorBgLayout: token('bg'), colorBgContainer: token('surface'),
      colorBorder: token('border-strong'), borderRadius: Number.parseInt(token('radius-md') ?? '6', 10),
      fontFamily: 'system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    },
    components: {
      Layout: { siderBg: token('sidebar-bg'), headerBg: token('surface'), bodyBg: token('bg') },
      Menu: {
        darkItemBg: token('sidebar-bg'), darkSubMenuItemBg: token('sidebar-bg'),
        darkItemColor: token('sidebar-muted'), darkItemHoverBg: token('sidebar-hover'),
        darkItemSelectedBg: token('sidebar-active'), darkItemSelectedColor: token('sidebar-text'),
      },
    },
  }
}
export function AdminTheme({ children }: { children: ReactNode }) {
  const [theme] = useState(createAdminTheme)
  return <ConfigProvider locale={zhCN} theme={theme}><App>{children}</App></ConfigProvider>
}
