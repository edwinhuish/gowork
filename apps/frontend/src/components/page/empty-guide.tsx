import { Empty, Space, Typography } from 'antd'
import type { ReactNode } from 'react'

/**
 * 空态引导。
 *
 * 空列表不是错误：缺的是一个「下一步做什么」。antd 的 Empty 只给一句描述，
 * 这里补上标题、口径说明与一个动作按钮，三处空态（表格、卡片视图、工作台）
 * 才长得一样。
 */
export function EmptyGuide({
  title = '还没有数据',
  description,
  action,
}: {
  title?: ReactNode
  description?: ReactNode
  action?: ReactNode
}) {
  return (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={
        <Space direction="vertical" size={4} className="empty-guide">
          <Typography.Text strong>{title}</Typography.Text>
          {description ? (
            <Typography.Text type="secondary" className="empty-guide-desc">
              {description}
            </Typography.Text>
          ) : null}
        </Space>
      }
    >
      {action}
    </Empty>
  )
}
