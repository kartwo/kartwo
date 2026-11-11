// 后台连接提示 / Admin Connection Notice
// 功能：首次配置阶段提示非本机 HTTP，避免为提示提前加载整套认证表单
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:08:19

import { Alert } from 'antd'
export function InsecureNotice() {
  const insecure = location.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]', '::1'].includes(location.hostname)
  return insecure ? <Alert type="warning" showIcon title="当前连接没有加密" description="这是配置阶段的临时状态。域名和证书配好后，请使用 HTTPS 网址访问后台。" /> : null
}
