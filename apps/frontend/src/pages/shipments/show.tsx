import { useState } from 'react'
import { useInvalidate, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import { Button, Card, Descriptions, message, Popconfirm, Space, Table, Tag } from 'antd'

import { shipmentApi } from '@/api/shipments'
import { Empty, Mono } from '@/components/fields'
import { orderStatusLabel } from '@/constants/catalog'
import type { Shipment, ShipmentAllocation } from '@/types'
import { formatDate, formatQty, formatTime } from '@/utils/format'

import { ChargesPanel } from './charge-panel'
import { CustomsPanel } from './customs-panel'
import { LogisticsCard } from './logistics-card'
import { ProfitCard } from './profit-card'

interface StatusAction {
  label: string
  next: number
  primary?: boolean
  danger?: boolean
}

/**
 * 当前状态能走的下一步。
 *
 * 与后端状态机一致，但只用来"少给几个按钮"——能不能跳由后端判定。
 * 出货单到"已出货"之后不再允许取消：货已经在路上，要冲出得走退货单。
 */
function nextActions(status: number): StatusAction[] {
  switch (status) {
    case 10:
      return [
        { label: '确认出货单', next: 20, primary: true },
        { label: '取消', next: 90, danger: true },
      ]
    case 20:
      return [
        { label: '开始备货', next: 30, primary: true },
        { label: '取消', next: 90, danger: true },
      ]
    case 30:
      return [
        { label: '标记已出货', next: 40, primary: true },
        { label: '取消', next: 90, danger: true },
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

export function ShipmentShow() {
  const { result, query } = useShow<Shipment>({ resource: 'shipments' })
  const invalidate = useInvalidate()
  const [acting, setActing] = useState(false)

  const s = result
  const actions = s ? nextActions(s.status) : []
  const totalQty = (s?.allocations ?? []).reduce((sum, a) => sum + a.qty, 0)

  const changeStatus = async (next: number, okText: string) => {
    if (!s) return
    setActing(true)
    try {
      await shipmentApi.updateStatus(s.id, next)
      message.success(okText)
      await invalidate({ resource: 'shipments', invalidates: ['detail', 'list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActing(false)
    }
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={s ? `出货单 ${s.doc_no}` : '出货单详情'}
      headerButtons={
        <Space wrap>
          {s?.status === 10 && <EditButton resource="shipments" recordItemId={s.id} />}
          {actions.map((a) =>
            a.danger ? (
              <Popconfirm
                key={a.next}
                title="取消这张出货单？"
                description="取消后这部分数量会还给对应的销售订单行，可以重新出货。"
                okText="确认取消"
                cancelText="返回"
                onConfirm={() => changeStatus(a.next, '出货单已取消')}
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
                onClick={() => changeStatus(a.next, `出货单已标记为${orderStatusLabel(a.next)}`)}
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
          {
            key: 'doc_no',
            label: '单号',
            children: <span className="cell-mono">{s?.doc_no}</span>,
          },
          {
            key: 'status',
            label: '状态',
            children: s ? (
              <Tag color={statusColor(s.status)}>{orderStatusLabel(s.status)}</Tag>
            ) : (
              <Empty />
            ),
          },
          {
            key: 'ship_date',
            label: '出货日',
            children: s ? formatDate(s.ship_date) : <Empty />,
          },
          {
            key: 'total',
            label: '合计数量',
            children: s ? `${(s.allocations ?? []).length} 行 · 共 ${formatQty(totalQty)}` : <Empty />,
          },
          { key: 'remark', label: '备注', span: 2, children: s?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(s?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(s?.updated_at) },
        ]}
      />

      <Card title="出货明细" size="small" style={{ marginTop: 16 }}>
        <Table
          rowKey="id"
          size="small"
          dataSource={s?.allocations ?? []}
          pagination={false}
          scroll={{ x: 900 }}
          locale={{ emptyText: '没有明细' }}
        >
          <Table.Column
            dataIndex="sales_order_doc_no"
            title="来源订单"
            width={160}
            render={(value: string) => (value ? <Mono>{value}</Mono> : <Empty />)}
          />
          <Table.Column
            dataIndex="sku_code"
            title="SKU"
            width={140}
            render={(value: string) => <Mono>{value}</Mono>}
          />
          <Table.Column
            dataIndex="name_cn"
            title="品名 / 规格"
            render={(value: string, record: ShipmentAllocation) => (
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
            title="出货数量"
            width={110}
            align="right"
            render={(value: number) => formatQty(value)}
          />
          <Table.Column
            dataIndex="remark"
            title="备注"
            render={(value: string) => value || <Empty />}
          />
        </Table>
      </Card>

      {/* 物流节点就地编辑：提单号与船期订舱后才有，那时单子早已确认。 */}
      {s ? <LogisticsCard shipment={s} /> : null}

      {/* 报关单一票多张：分票申报与退单重报都是新的一条，旧的留着。 */}
      {s?.id ? <CustomsPanel shipmentId={s.id} /> : null}

      {/* 运费杂费是这一票的成本：海运费按票收，摊到订单行上要先定分摊规则，
          规则一改历史数字全变，所以挂在票上。 */}
      {s?.id ? <ChargesPanel shipmentId={s.id} /> : null}

      {/* 毛利放在费用面板之后：先看完费用怎么来的，再看它把毛利压掉多少。 */}
      {s?.id ? <ProfitCard shipmentId={s.id} status={s.status} /> : null}
    </Show>
  )
}
