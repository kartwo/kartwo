// 关于 Kartwo / About Kartwo
// 功能：保留 MIT 许可和自行部署使用提示
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

import { Card, Typography } from 'antd'
export default function About() {
  return <div className="settings-page"><Card title="关于 Kartwo · 开源许可与使用提示">
    <Typography.Paragraph>Kartwo 是依据 MIT 许可证提供的通用开源电商软件。自行部署和运营网站的使用者，应确保其商品、交易、支付、网站内容及个人信息处理符合适用法律。Kartwo 项目维护者不参与第三方自行部署实例的经营或交易。</Typography.Paragraph>
    <Typography.Paragraph>本提示不修改 MIT 许可证，也不免除任何一方依法应承担的责任。</Typography.Paragraph>
    <a href="https://github.com/kartwo/kartwo/blob/main/LICENSE" target="_blank" rel="noopener noreferrer">查看 MIT 许可证</a>
  </Card></div>
}
