import { useState } from 'react'
import { useInvalidate, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import { Button, Card, Descriptions, message, Popconfirm, Space, Table, Tag } from 'antd'

import { download } from '@/api/client'
import { purchaseOrderApi } from '@/api/purchase-orders'
import { Empty, Mono, Money } from '@/components/fields'
import { orderStatusLabel, tradeTermLabel } from '@/constants/catalog'
import type { PurchaseOrder, PurchaseOrderItem } from '@/types'
import { formatDate, formatTime } from '@/utils/format'

import { PurchaseMilestonePanel } from './milestone-panel'
import { InspectionPanel } from '@/components/inspection-panel'
import { PurchaseReceiptPanel } from '@/components/purchase-receipt-panel'

interface StatusAction {
  label: string
  next: number
  primary?: boolean
  danger?: boolean
}

/** 与销售订单同一套状态机与同一份"下一步"清单。 */
function nextActions(status: number): StatusAction[] {
  switch (status) {
    case 10:
      return [
        { label: '确认采购', next: 20, primary: true },
        { label: '取消订单', next: 90, danger: true },
      ]
    case 20:
      return [
        { label: '开始备货', next: 30, primary: true },
        { label: '取消订单', next: 90, danger: true },
      ]
    case 30:
      return [
        { label: '标记已到货', next: 40, primary: true },
        { label: '取消订单', next: 90, danger: true },
      ]
    case 40:
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

export function PurchaseShow() {
  const { result, query } = useShow<PurchaseOrder>({ resource: 'purchase-orders' })
  const invalidate = useInvalidate()
  const [acting, setActing] = useState(false)
  const [printing, setPrinting] = useState(false)

  const o = result
  const actions = o ? nextActions(o.status) : []

  // 这张单是发给供应商的排产依据：之前只能手工做一份 Excel 发出去，
  // 而手工那份的价格和系统里的订单对不上。
  const printPDF = async () => {
    if (!o) return
    setPrinting(true)
    try {
      await download(`/purchase-orders/${o.id}/pdf`, `${o.doc_no}-PO.pdf`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载失败')
    } finally {
      setPrinting(false)
    }
  }

  const changeStatus = async (next: number, okText: string) => {
    if (!o) return
    setActing(true)
    try {
      await purchaseOrderApi.updateStatus(o.id, next)
      message.success(okText)
      await invalidate({ resource: 'purchase-orders', invalidates: ['detail', 'list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActing(false)
    }
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={o ? `采购订单 ${o.doc_no}` : '采购订单详情'}
      headerButtons={
        <Space wrap>
          <Button onClick={printPDF} loading={printing}>
            打印 PDF
          </Button>
          {o?.status === 10 && <EditButton resource="purchase-orders" recordItemId={o.id} />}
          {actions.map((a) =>
            a.danger ? (
              <Popconfirm
                key={a.next}
                title="取消这张采购单？"
                description="取消是终态。已备料或已开工时请先与工厂确认。"
                okText="取消订单"
                cancelText="返回"
                onConfirm={() => changeStatus(a.next, '采购单已取消')}
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
                onClick={() => changeStatus(a.next, `采购单已标记为${orderStatusLabel(a.next)}`)}
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
          { key: 'supplier', label: '供应商', children: o?.supplier?.name || <Empty /> },
          {
            key: 'trade_term',
            label: '贸易术语',
            children: o?.trade_term ? <Tag>{tradeTermLabel(o.trade_term)}</Tag> : <Empty />,
          },
          { key: 'order_date', label: '下单日', children: o ? formatDate(o.order_date) : <Empty /> },
          {
            key: 'delivery_date',
            label: '承诺到货日',
            children: o?.delivery_date ? formatDate(o.delivery_date) : <Empty />,
          },
          {
            key: 'currency_code',
            label: '采购币种 / 汇率',
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
            label: '采购金额',
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
          scroll={{ x: 1000 }}
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
            render={(value: string, record: PurchaseOrderItem) => (
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
            render={(value: string) => Number(value).toLocaleString('zh-CN')}
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
            dataIndex="sales_order_item_id"
            title="关联"
            width={110}
            render={(value: number | null) =>
              value ? <Tag color="blue">以销定采</Tag> : <Empty />
            }
          />
        </Table>
      </Card>

      {/* 跟单节点（T-38）：货在工厂还是已经进仓、验货过没过，就看这张表。 */}
      {/* 收货在跟单节点之前：节点回答"货在工厂还是已进仓"，收货回答"进了多少"。 */}
      {o ? <PurchaseReceiptPanel orderId={o.id} items={o.items ?? []} /> : null}

      {/* 验货跟在收货之后：先到货、再验货，这也是实际发生的顺序。 */}
      {o ? <InspectionPanel orderId={o.id} items={o.items ?? []} /> : null}

      {o ? <PurchaseMilestonePanel order={o} /> : null}
    </Show>
  )
}
