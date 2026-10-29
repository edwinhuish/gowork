import { useCallback, useEffect, useState } from 'react'
import { useGetIdentity, useInvalidate, useNavigation, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import { Button, Card, Checkbox, Descriptions, message, Popconfirm, Space, Table, Tag, Tooltip } from 'antd'

import { salesOrderApi } from '@/api/sales-orders'
import { Empty, Mono, Money } from '@/components/fields'
import { orderStatusLabel, tradeTermLabel } from '@/constants/catalog'
import type { SalesOrder, SalesOrderChange, SalesOrderChangeLine, SalesOrderItem } from '@/types'

import { ChangeModal } from './change-modal'
import { CreditHint } from './credit-hint'
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

/**
 * 变更明细的「改前 → 改后」。
 *
 * 只写真的动了的那一格：数量与单价都列出来会让人以为两处都改了，
 * 而翻旧账时最想确认的恰恰是"到底动了哪个"。
 */
function ChangeLines({ lines }: { lines?: SalesOrderChangeLine[] }) {
  if (!lines?.length) return <span className="cell-sub">只改了交期</span>
  return (
    <div className="cell-stack">
      {lines.map((line) => {
        const qtyChanged = Number(line.before_qty) !== Number(line.after_qty)
        const priceChanged = Number(line.before_unit_price) !== Number(line.after_unit_price)
        return (
          <span key={line.id}>
            <Mono>{line.sku_code}</Mono>{' '}
            {qtyChanged && (
              <span>
                数量 {formatQty(line.before_qty)} → {formatQty(line.after_qty)}
              </span>
            )}
            {qtyChanged && priceChanged ? '，' : null}
            {priceChanged && (
              <span>
                单价 <Money value={line.before_unit_price} /> → <Money value={line.after_unit_price} />
              </span>
            )}
          </span>
        )
      })}
    </div>
  )
}

export function OrderShow() {
  const { result, query } = useShow<SalesOrder>({ resource: 'sales-orders' })
  const invalidate = useInvalidate()
  const { create } = useNavigation()
  const { data: identity } = useGetIdentity<{ role?: string }>()
  const [acting, setActing] = useState(false)
  // 勾了才带 force_credit：默认不勾，防止"手滑确认"变成默认绕过额度。
  const [forceCredit, setForceCredit] = useState(false)
  const [changeOpen, setChangeOpen] = useState(false)
  const [changes, setChanges] = useState<SalesOrderChange[]>([])
  const [changesLoading, setChangesLoading] = useState(false)
  const isAdmin = identity?.role === 'admin'

  const o = result
  const actions = o ? nextActions(o.status) : []
  const orderId = o?.id

  const loadChanges = useCallback(async (id: number) => {
    setChangesLoading(true)
    try {
      setChanges(await salesOrderApi.listChanges(id))
    } catch (err) {
      // 历史取不到不该挡住看单据：订单本身仍然可读，提示一次就够。
      message.error(err instanceof Error ? err.message : '变更历史加载失败')
    } finally {
      setChangesLoading(false)
    }
  }, [])

  useEffect(() => {
    if (orderId) void loadChanges(orderId)
  }, [orderId, loadChanges])

  const afterChange = async () => {
    if (!orderId) return
    await invalidate({ resource: 'sales-orders', invalidates: ['detail', 'list'] })
    await loadChanges(orderId)
  }

  const changeStatus = async (next: number, okText: string, force = false) => {
    if (!o) return
    setActing(true)
    try {
      await salesOrderApi.updateStatus(o.id, next, force)
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
          {/* 已确认的订单不能直接改：改价 / 改量 / 改交期都走变更单，留下改前改后与原因。 */}
          {(o?.status === 20 || o?.status === 30) && (
            <Tooltip title="改价 / 改量 / 改交期都会留痕：原因、改前改后、操作人。不能增删明细行。">
              <Button onClick={() => setChangeOpen(true)}>变更订单</Button>
            </Tooltip>
          )}
          {/* 已确认、备货中才谈得上出货：草稿还没对客户承诺，已出货的再出要新开一票。 */}
          {(o?.status === 20 || o?.status === 30) && (
            <Button onClick={() => create('shipments')}>按这张单出货</Button>
          )}
          {/* 强制放行只对 admin 露出；后端仍会再看一次身份，勾了也不等于一定放行。 */}
          {o?.status === 10 && isAdmin && (
            <Tooltip title="越过客户信用额度确认。放行会记录操作人与超额金额。">
              <Checkbox
                checked={forceCredit}
                onChange={(e) => setForceCredit(e.target.checked)}
              >
                强制放行额度
              </Checkbox>
            </Tooltip>
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
                onClick={() =>
                  changeStatus(
                    a.next,
                    `订单已标记为${orderStatusLabel(a.next)}`,
                    a.next === 20 && forceCredit,
                  )
                }
              >
                {a.label}
              </Button>
            ),
          )}
        </Space>
      }
    >
      {/* 还没结清的单据才谈额度：已完成的对账已闭环，已取消的不占额度。 */}
      {o && o.status >= 10 && o.status <= 40 && <CreditHint partnerId={o.partner_id} />}

      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="middle"
        items={[
          { key: 'doc_no', label: '单号', children: <span className="cell-mono">{o?.doc_no}</span> },
          {
            key: 'base_no',
            label: '案号',
            // 与来源报价共用：同一笔生意在两张单上看到的是同一个号。
            children: o?.base_no ? <span className="cell-mono">{o.base_no}</span> : <Empty />,
          },
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
            render={(value: string) => Number(value).toLocaleString('zh-CN')}
          />
          <Table.Column
            dataIndex="shipped_qty"
            title="已出"
            width={90}
            align="right"
            render={(value?: string) => (value === undefined ? <Empty /> : formatQty(value))}
          />
          <Table.Column
            dataIndex="unshipped_qty"
            title="未出"
            width={90}
            align="right"
            render={(value?: string) => (value === undefined ? <Empty /> : formatQty(value))}
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

      {/* 没改过就不占版面：这张卡片的意义全在"改过什么"，空表只是多一块空白。 */}
      {changes.length > 0 && (
        <Card
          title="变更历史"
          size="small"
          style={{ marginTop: 16 }}
          extra={<span className="cell-sub">共 {changes.length} 次，最近一次在最上面</span>}
        >
          <Table
            rowKey="id"
            size="small"
            loading={changesLoading}
            dataSource={changes}
            pagination={false}
            scroll={{ x: 1100 }}
          >
            <Table.Column
              dataIndex="seq"
              title="次数"
              width={70}
              render={(value: number) => <Tag>#{value}</Tag>}
            />
            <Table.Column
              dataIndex="doc_no"
              title="变更单号"
              width={170}
              render={(value: string) => <Mono>{value}</Mono>}
            />
            <Table.Column
              dataIndex="created_at"
              title="时间"
              width={170}
              render={(value: string) => formatTime(value)}
            />
            <Table.Column
              dataIndex="operator"
              title="操作人"
              width={110}
              render={(value: string) => value || <Empty />}
            />
            <Table.Column
              dataIndex="reason"
              title="原因"
              render={(value: string) => value || <Empty />}
            />
            <Table.Column
              title="订单总额"
              width={200}
              align="right"
              render={(_, record: SalesOrderChange) => (
                <Space size={4}>
                  <Money value={record.before_total} />
                  <span>→</span>
                  <Money value={record.after_total} />
                </Space>
              )}
            />
            <Table.Column
              title="交期"
              width={190}
              render={(_, record: SalesOrderChange) =>
                // 两边都空 = 一直没定交期，显示一个短横，而不是「— → —」。
                !record.before_delivery_date && !record.after_delivery_date ? (
                  <Empty />
                ) : (
                  `${formatDate(record.before_delivery_date ?? undefined)} → ${formatDate(
                    record.after_delivery_date ?? undefined,
                  )}`
                )
              }
            />
            <Table.Column
              title="明细改动"
              render={(_, record: SalesOrderChange) => <ChangeLines lines={record.lines} />}
            />
          </Table>
        </Card>
      )}

      {o && (
        <ChangeModal
          open={changeOpen}
          order={o}
          onClose={() => setChangeOpen(false)}
          onChanged={afterChange}
        />
      )}
    </Show>
  )
}
