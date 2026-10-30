import { Flex, Space, Typography } from 'antd'
import type { ReactNode } from 'react'

/**
 * 页面抬头：标题 + 一句话口径 + 右侧动作。
 *
 * refine 的 List / Show 自带标题区，但没有「这页在算什么」的位置。
 * 报表页（账龄、汇兑损益、回款）与工作台都不是 refine 外壳，需要一块自己的抬头；
 * 统一成同一个组件之后，三种页面的标题节奏才一致。
 */
export function PageHeader({
  title,
  description,
  tags,
  extra,
}: {
  title: ReactNode
  description?: ReactNode
  /** 标题旁的状态/范围标签。 */
  tags?: ReactNode
  extra?: ReactNode
}) {
  return (
    <Flex className="page-head" wrap align="flex-start" justify="space-between" gap={12}>
      <div className="page-head-text">
        <Space size={8} wrap align="center">
          <Typography.Title level={4} className="page-head-title">
            {title}
          </Typography.Title>
          {tags}
        </Space>
        {description ? (
          <Typography.Text type="secondary" className="page-head-desc">
            {description}
          </Typography.Text>
        ) : null}
      </div>
      {extra ? (
        <Space wrap size={8} className="page-head-extra">
          {extra}
        </Space>
      ) : null}
    </Flex>
  )
}
