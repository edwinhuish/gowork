import { useState } from 'react'
import type { FormInstance } from 'antd'
import { useList } from '@refinedev/core'
import { Button, message, Select, Space, Typography } from 'antd'

import { http } from '@/api/client'
import type { LineItemRow } from '@/components/line-items-editor'
import type { SalesOrder } from '@/types'

/**
 * 从销售订单带出采购明细（以销定采）。
 *
 * 不带采购价：采购价是谈出来的，从销售价推不出来。带过来的是「买什么、买多少、
 * 为哪一行备的」，单价留空由人填——这比默认塞一个销售价再让人改更安全。
 *
 * 只排除已取消的销售单；草稿单也允许（先备货后确认是常见的）。
 */
export function SalesOrderImport({ form }: { form: FormInstance }) {
  const [selected, setSelected] = useState<number>()
  const [loading, setLoading] = useState(false)

  const { query } = useList<SalesOrder>({
    resource: 'sales-orders',
    pagination: { mode: 'off' },
  })

  const options = (query.data?.data ?? [])
    .filter((o) => o.status !== 90)
    .map((o) => ({
      value: o.id,
      label: `${o.doc_no} · ${o.partner?.name ?? '—'}`,
    }))

  const run = async () => {
    if (!selected) {
      message.warning('先选一张销售订单')
      return
    }
    setLoading(true)
    try {
      const so = await http.get<SalesOrder>(`/sales-orders/${selected}`)
      const rows: LineItemRow[] = (so.items ?? []).map((it) => ({
        product_id: it.product_id,
        qty: it.qty,
        unit_price: '0',
        sales_order_item_id: it.id,
      }))
      if (rows.length === 0) {
        message.warning('这张销售订单没有明细')
        return
      }
      // 追加而不是覆盖：已经手填的行不该因为导一次单被清掉。
      const current = (form.getFieldValue('items') as LineItemRow[] | undefined) ?? []
      form.setFieldValue('items', [...current, ...rows])
      message.success(`已带出 ${rows.length} 行，请补填采购单价`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '导入失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Space wrap style={{ marginBottom: 12 }}>
      <Typography.Text type="secondary">以销定采：</Typography.Text>
      <Select
        placeholder="选择销售订单"
        allowClear
        showSearch
        style={{ width: 320 }}
        value={selected}
        onChange={(v) => setSelected(v)}
        options={options}
        loading={query.isLoading}
        filterOption={(input, option) =>
          String(option?.label ?? '')
            .toLowerCase()
            .includes(input.toLowerCase())
        }
      />
      <Button onClick={run} loading={loading}>
        带出明细
      </Button>
    </Space>
  )
}
