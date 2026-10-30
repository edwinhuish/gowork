import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { useGetIdentity, useInvalidate, useList, useNavigation, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import {
  Button,
  Card,
  Checkbox,
  Descriptions,
  message,
  Popconfirm,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'

import { download } from '@/api/client'
import { salesOrderApi } from '@/api/sales-orders'
import { AttachmentPanel } from '@/components/attachment-panel'
import { DocChain } from '@/components/doc-chain'
import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import {
  ORDER_STATUS_OPTIONS,
  SETTLEMENT_STATUS_OPTIONS,
  orderStatusLabel,
  tradeTermLabel,
} from '@/constants/catalog'
import type { StatusTone } from '@/constants/catalog'
import type {
  SalesOrder,
  SalesOrderChange,
  SalesOrderChangeLine,
  SalesOrderItem,
  Settlement,
  Shipment,
} from '@/types'

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

/** 底部「关联单据」表的行：出货与应收用的是两本不同的状态字典，所以字典跟着行走。 */
interface LinkedDocRow {
  status: number
  statusOptions: readonly { value: string | number; label: string; tone?: StatusTone }[]
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
  const [printing, setPrinting] = useState(false)
  const isAdmin = identity?.role === 'admin'

  const o = result
  const actions = o ? nextActions(o.status) : []
  const orderId = o?.id
  const navigate = useNavigate()

  /*
   * 这张订单的上下游各查一次：后端早就支持按 sales_order_id 查出货、
   * 按 source_type + source_id 查应收（10 = 销售订单），所以不新增接口。
   * 有了它，"这张单出了几次货、开了几张票"不用再回列表筛。
   */
  const { result: shipmentResult } = useList<Shipment>({
    resource: 'shipments',
    filters: [{ field: 'sales_order_id', operator: 'eq', value: orderId }],
    pagination: { mode: 'off' },
    queryOptions: { enabled: !!orderId },
  })
  const { result: receivableResult } = useList<Settlement>({
    resource: 'receivables',
    filters: [
      { field: 'source_type', operator: 'eq', value: 10 },
      { field: 'source_id', operator: 'eq', value: orderId },
    ],
    pagination: { mode: 'off' },
    queryOptions: { enabled: !!orderId },
  })
  const linkedShipments = shipmentResult?.data ?? []
  const linkedReceivables = receivableResult?.data ?? []

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

  // 草稿也能打印：客户常常先要一份书面确认再去走内部审批。
  const printPDF = async () => {
    if (!o) return
    setPrinting(true)
    try {
      await download(`/sales-orders/${o.id}/pdf`, `${o.doc_no}-SO.pdf`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载失败')
    } finally {
      setPrinting(false)
    }
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
          <Button onClick={printPDF} loading={printing}>
            打印 PDF
          </Button>
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
      <DocChain
        current="order"
        docs={{
          order: { docNo: o?.doc_no },
          shipment: linkedShipments.length
            ? {
                docNo:
                  linkedShipments.length > 1
                    ? `${linkedShipments[0].doc_no} 等 ${linkedShipments.length} 票`
                    : linkedShipments[0].doc_no,
                href: `/shipments/show/${linkedShipments[0].id}`,
              }
            : undefined,
          receivable: linkedReceivables.length
            ? {
                docNo:
                  linkedReceivables.length > 1
                    ? `${linkedReceivables[0].doc_no} 等 ${linkedReceivables.length} 张`
                    : linkedReceivables[0].doc_no,
                href: `/receivables/show/${linkedReceivables[0].id}`,
              }
            : undefined,
        }}
      />

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
              <OptionTag value={o.status} options={ORDER_STATUS_OPTIONS} />
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

      <Card
        title="关联单据"
        size="small"
        style={{ marginTop: 16 }}
        extra={<span className="cell-sub">出货与应收都由这张订单产生，点单号进详情</span>}
      >
        <Table
          rowKey="key"
          size="small"
          pagination={false}
          scroll={{ x: 720 }}
          dataSource={[
            ...linkedShipments.map((row) => ({
              key: `s-${row.id}`,
              kind: '出货',
              docNo: row.doc_no,
              date: row.ship_date,
              amount: '',
              status: row.status,
              statusOptions: ORDER_STATUS_OPTIONS,
              href: `/shipments/show/${row.id}`,
            })),
            ...linkedReceivables.map((row) => ({
              key: `r-${row.id}`,
              kind: '应收',
              docNo: row.doc_no,
              date: row.due_date,
              amount: row.amount_base,
              status: row.status,
              statusOptions: SETTLEMENT_STATUS_OPTIONS,
              href: `/receivables/show/${row.id}`,
            })),
          ]}
          locale={{ emptyText: '这张订单还没有出货、也没有开票' }}
        >
          <Table.Column
            dataIndex="kind"
            title="类型"
            width={80}
            // 「出货 / 应收」是这张单关联了什么，不是状态，所以中性标签。
            render={(value: string) => <Tag bordered={false}>{value}</Tag>}
          />
          <Table.Column
            dataIndex="docNo"
            title="单号"
            width={180}
            render={(value: string, row: { href: string }) => (
              <Typography.Link onClick={() => navigate(row.href)}>
                <Mono>{value}</Mono>
              </Typography.Link>
            )}
          />
          <Table.Column dataIndex="date" title="日期" width={120} />
          <Table.Column
            dataIndex="amount"
            title="金额（本币）"
            align="right"
            width={140}
            render={(value: string) => (value ? <Money value={value} /> : <Empty />)}
          />
          <Table.Column
            dataIndex="status"
            title="状态"
            width={100}
            render={(_: unknown, row: LinkedDocRow) => (
              <OptionTag value={row.status} options={row.statusOptions} />
            )}
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
      {/* T-63：后端早就能挂附件，前端入口不全，等于大部分能力用不上。
          客户 PO 与订单确认件挂在这里，比挂在主体上更有用——每家客户一年有几十张单。 */}
      {o?.id ? (
        <AttachmentPanel entityType="sales_order" entityId={o.id} title="客户 PO 与订单确认件" />
      ) : null}

    </Show>
  )
}
