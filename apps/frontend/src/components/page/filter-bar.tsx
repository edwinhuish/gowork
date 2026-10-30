import { Card, Flex, Space } from 'antd'
import type { ReactNode } from 'react'

/**
 * 报表类页面的筛选条。
 *
 * 报表页不是 refine 的列表，不套 useTable / PageList，但它们同样有一个
 * 「条件 + 口径」的横条。改造前每个报表页自己写 Card + Space：
 * 内距不一致、窄屏不换行、条件与说明混在一行里。收成一个组件后，
 * 条件在左、口径与截止日在右，四张报表长得一样。
 */
export function FilterBar({
  children,
  extra,
}: {
  /** 条件控件：Radio.Group、日期输入、口径选择等。 */
  children: ReactNode
  /** 右侧的口径说明或截止日。 */
  extra?: ReactNode
}) {
  return (
    <Card size="small" variant="outlined" className="filter-bar">
      <Flex wrap align="center" justify="space-between" gap={12}>
        <Flex wrap align="center" gap={12}>
          {children}
        </Flex>
        {extra ? (
          <Space wrap size={8} className="filter-bar-extra">
            {extra}
          </Space>
        ) : null}
      </Flex>
    </Card>
  )
}
