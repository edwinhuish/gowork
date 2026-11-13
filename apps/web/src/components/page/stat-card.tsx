import { Card, Flex, Statistic, theme } from 'antd'
import type { ReactNode } from 'react'

export type StatTone = 'primary' | 'success' | 'warning' | 'danger' | 'info'

/**
 * 指标卡：标题 + 图标 + 数字 + 一句口径。
 *
 * 改造前工作台里有一个局部 KpiCard，红/橙两档颜色写死成 #dc2626 / #d97706；
 * 报表页又各自用 Card + Statistic 拼一遍。收成一个组件后，颜色只从
 * antd 的语义 token 取，暗色模式下不用逐个改。
 */
export function StatCard({
  title,
  value,
  prefix,
  suffix,
  precision,
  hint,
  icon,
  tone = 'primary',
  loading,
  onClick,
}: {
  title: ReactNode
  value?: ReactNode
  prefix?: ReactNode
  suffix?: ReactNode
  precision?: number
  hint?: ReactNode
  icon?: ReactNode
  tone?: StatTone
  loading?: boolean
  onClick?: () => void
}) {
  const { token } = theme.useToken()
  const color = {
    primary: token.colorPrimary,
    success: token.colorSuccess,
    warning: token.colorWarning,
    danger: token.colorError,
    info: token.colorInfo,
  }[tone]

  return (
    <Card
      size="small"
      className="stat-card"
      loading={loading}
      hoverable={Boolean(onClick)}
      onClick={onClick}
    >
      <Flex align="flex-start" justify="space-between" gap={12}>
        {/* 金额是 <Money> 这类节点而不是数字：Statistic 只在拿到 number 时才
            做千分位与精度格式化，节点直接交给 valueRender，别转成字符串再丢格式。 */}
        {typeof value === 'number' ? (
          <Statistic title={title} value={value} precision={precision} prefix={prefix} suffix={suffix} />
        ) : (
          <Statistic title={title} valueRender={() => value} />
        )}
        {icon ? (
          <span className="stat-icon" style={{ color, background: token.colorFillTertiary }}>
            {icon}
          </span>
        ) : null}
      </Flex>
      {hint ? (
        <span className="stat-hint" style={{ color: token.colorTextTertiary }}>
          {hint}
        </span>
      ) : null}
    </Card>
  )
}
