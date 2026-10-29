import { useState } from 'react'
import type { CrudFilters } from '@refinedev/core'
import { useInvalidate } from '@refinedev/core'
import { List, useTable } from '@refinedev/antd'
import { Button, Form, Input, Space, Table, Typography, message } from 'antd'

import { stockApi } from '@/api/stock'
import { Empty, Mono } from '@/components/fields'
import type { StockSnapshotRow } from '@/types'
import { formatDate, formatQty, formatTime } from '@/utils/format'

/**
 * 当前库存。
 *
 * 只有读与"重算"：库存由流水推出来，界面上没有直接改它的入口。
 * 要调数就补一条流水（期初 / 盘盈亏 / 其它），这样每一次变化都留得下痕迹。
 */
export function StockList() {
  const invalidate = useInvalidate()
  const [busy, setBusy] = useState(false)

  const { tableProps, searchFormProps } = useTable<StockSnapshotRow>({
    resource: 'stock-snapshots',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as { keyword?: string }
      if (!v.keyword?.trim()) return []
      return [{ field: 'keyword', operator: 'contains', value: v.keyword.trim() }]
    },
  })

  /** productId 留空 = 重算全部。 */
  const recalc = async (productId?: number) => {
    setBusy(true)
    try {
      const res = await stockApi.recalc(productId)
      const diff = res.mismatched.length
      message.success(
        `已按流水重算 ${res.recalculated} 个产品` +
          (diff > 0
            ? `，其中 ${diff} 个此前与流水不一致，已修正`
            : '，未发现差异'),
      )
      await invalidate({ resource: 'stock-snapshots', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '重算失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <List
      headerButtons={
        <Button type="primary" loading={busy} onClick={() => recalc()}>
          按流水重算
        </Button>
      }
    >
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索 SKU / 品名" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">
              查询
            </Button>
            <Button
              onClick={() => {
                searchFormProps.form?.resetFields()
                searchFormProps.form?.submit()
              }}
            >
              重置
            </Button>
          </Space>
        </Form.Item>
      </Form>

      <Table {...tableProps} rowKey="product_id" scroll={{ x: 900 }}>
        <Table.Column
          dataIndex="sku_code"
          title="SKU"
          width={160}
          render={(value: string) => (value ? <Mono>{value}</Mono> : <Empty />)}
        />
        <Table.Column
          dataIndex="name_cn"
          title="品名 / 规格"
          render={(value: string, record: StockSnapshotRow) => (
            <div className="cell-stack">
              <span className="cell-strong">{value || <Empty />}</span>
              {record.spec && <span className="cell-sub">{record.spec}</span>}
            </div>
          )}
        />
        <Table.Column
          dataIndex="unit"
          title="单位"
          width={70}
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          dataIndex="qty"
          title="当前库存"
          width={130}
          align="right"
          // 数量是定点字符串：这里的正负判断只用于着色。
          render={(value: string) => (
            <span className="cell-mono">
              {/* 负库存不是错误：先出货后补入库单在直发业务里很正常，但要显眼。 */}
              {Number(value) < 0 ? (
                <Typography.Text type="danger">{formatQty(value)}</Typography.Text>
              ) : (
                formatQty(value)
              )}
            </span>
          )}
        />
        <Table.Column
          dataIndex="last_occurred_at"
          title="最后变动"
          width={130}
          render={(value?: string | null) => (value ? formatDate(value) : <Empty />)}
        />
        <Table.Column
          dataIndex="updated_at"
          title="快照更新于"
          width={180}
          render={(value: string) => formatTime(value)}
        />
        <Table.Column
          title="操作"
          width={110}
          render={(_, record: StockSnapshotRow) => (
            <Button type="link" loading={busy} onClick={() => recalc(record.product_id)}>
              重算
            </Button>
          )}
        />
      </Table>
    </List>
  )
}
