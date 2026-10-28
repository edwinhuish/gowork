import { useState } from 'react'
import { useInvalidate, useNavigation, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import { Button, Card, Descriptions, message, Popconfirm, Space, Table, Tag } from 'antd'

import { salesOrderApi } from '@/api/sales-orders'
import { Empty, Mono, Money } from '@/components/fields'
import { orderStatusLabel, tradeTermLabel } from '@/constants/catalog'
import type { SalesOrder, SalesOrderItem } from '@/types'
import { formatDate, formatQty, formatTime } from '@/utils/format'

interface StatusAction {
  label: string
  next: number
  primary?: boolean
  danger?: boolean
}

/**
 * 当前状态能走的下一步。
 *
 * 与后端状态机保持一致，但这里只用来"少给几个按钮"——能不能跳最终由后端判定，
 * 前端放开一点不会造成脏数据，只会得到一条解释清楚的 409。
 */
function nextActions(status: number): StatusAction[] {
  switch (status) {
    case 10:
      return [
        { label: '确认订单', next: 20, primary: true },
        { label: '取消订单', next: 90, danger: true },
      ]
    case 20:
      return [
        { label: '开始备货', next: 30, primary: true },
        { label: '取消订单', next: 90, danger: true },
      ]
    case 30:
      return [
        { label: '标记已出货', next: 40, primary: true },
        { label: '取消订单', next: 90, danger: true },
      ]
    case 40:
      // 已出货之后不再允许取消：货已经在路上，取消只会让对账多出一个悬空单据。
      return [{ label: '标记已完成', next: 50, primary: true }]
    default:
      return []
  }
}

function statusColor(value: number): string {
  if (value === 20) return 'success'
  if (value === 30) return 'processing'
  if (value === 40) return 'warning'
  if (value === 50) return 'default'
  if (value === 90) return 'error'
  return 'default'
}

export function OrderShow() {
  const { result, query } = useShow<SalesOrder>({ resource: 'sales-orders' })
  const invalidate = useInvalidate()
  const { create } = useNavigation()
  const [acting, setActing] = useState(false)

  const o = result
  const actions = o ? nextActions(o.status) : []

  const changeStatus = async (next: number, okText: string) => {
    if (!o) return
    setActing(true)
    try {
      await salesOrderApi.updateStatus(o.id, next)
      message.success(okText)
      await invalidate({ resource: 'sales-orders', invalidates: ['detail', 'list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActing(false)
    }
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={o ? `销售订单 ${o.doc_no}` : '销售订单详情'}
      headerButtons={
        <Space wrap>
          {o?.status === 10 && <EditButton resource="sales-orders" recordItemId={o.id} />}
          {/* 已确认、备货中才谈得上出货：草稿还没对客户承诺，已出货的再出要新开一票。 */}
          {(o?.status === 20 || o?.status === 30) && (
            <Button onClick={() => create('shipments')}>按这张单出货</Button>
          )}
          {actions.map((a) =>
            a.danger ? (
              <Popconfirm
                key={a.next}
                title="取消这张订单？"
                description="取消是终态。已排产或已备料时请先与工厂确认。"
                okText="取消订单"
                cancelText="返回"
                onConfirm={() => changeStatus(a.next, '订单已取消')}
              >
                <Button danger loading={acting}>
                  {a.label}
                </Button>
              </Popconfirm>
            ) : (
              <Button
                key={a.next}
                type={a.primary ? 'primary' : 'default'}
                loading={acting}
                onClick={() => changeStatus(a.next, `订单已标记为${orderStatusLabel(a.next)}`)}
              >
                {a.label}
              </Button>
            ),
          )}
        </Space>
      }
    >
      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="middle"
        items={[
          { key: 'doc_no', label: '单号', children: <span className="cell-mono">{o?.doc_no}</span> },
          {
            key: 'status',
            label: '状态',
            children: o ? (
              <Tag color={statusColor(o.status)}>{orderStatusLabel(o.status)}</Tag>
            ) : (
              <Empty />
            ),
          },
          { key: 'partner', label: '客户', children: o?.partner?.name || <Empty /> },
          {
            key: 'trade_term',
            label: '贸易术语',
            children: o?.trade_term ? <Tag>{tradeTermLabel(o.trade_term)}</Tag> : <Empty />,
          },
          {
            key: 'order_date',
            label: '下单日',
            children: o ? formatDate(o.order_date) : <Empty />,
          },
          {
            key: 'delivery_date',
            label: '约定交期',
            children: o?.delivery_date ? formatDate(o.delivery_date) : <Empty />,
          },
          {
            key: 'currency_code',
            label: '成交币种 / 汇率',
            children: o ? (
              <Space size={6}>
                <Tag>{o.currency_code}</Tag>
                <span className="cell-mono">{o.exchange_rate}</span>
              </Space>
            ) : (
              <Empty />
            ),
          },
          {
            key: 'amount_total',
            label: '订单金额',
            children: o ? <Money value={o.amount_total} /> : <Empty />,
          },
          {
            key: 'amount_base_total',
            label: '折本币',
            children: o ? <Money value={o.amount_base_total} /> : <Empty />,
          },
          { key: 'remark', label: '备注', span: 2, children: o?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(o?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(o?.updated_at) },
        ]}
      />

      <Card title="明细" size="small" style={{ marginTop: 16 }}>
        <Table
          rowKey="id"
          size="small"
          dataSource={o?.items ?? []}
          pagination={false}
          scroll={{ x: 900 }}
          locale={{ emptyText: '没有明细' }}
        >
          <Table.Column
            dataIndex="sku_code"
            title="SKU"
            width={140}
            render={(value: string) => <Mono>{value}</Mono>}
          />
          <Table.Column
            dataIndex="name_cn"
            title="品名 / 规格"
            render={(value: string, record: SalesOrderItem) => (
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
            title="数量"
            width={90}
            align="right"
            render={(value: number) => value.toLocaleString('zh-CN')}
          />
          <Table.Column
            dataIndex="shipped_qty"
            title="已出"
            width={90}
            align="right"
            render={(value?: number) => (value === undefined ? <Empty /> : formatQty(value))}
          />
          <Table.Column
            dataIndex="unshipped_qty"
            title="未出"
            width={90}
            align="right"
            render={(value?: number) => (value === undefined ? <Empty /> : formatQty(value))}
          />
          <Table.Column
            dataIndex="unit_price"
            title="单价"
            width={110}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="amount"
            title="金额"
            width={130}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="amount_base"
            title="折本币"
            width={130}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="remark"
            title="备注"
            render={(value: string) => value || <Empty />}
          />
        </Table>
      </Card>
    </Show>
  )
}
