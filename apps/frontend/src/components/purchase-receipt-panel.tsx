import { useCallback, useEffect, useState } from 'react'
import { Button, Card, DatePicker, Input, InputNumber, Modal, Popconfirm, Space, Table, Tag, message } from 'antd'

import {
  purchaseReceiptApi,
  type PurchaseReceipt,
  type PurchaseReceiptItem,
  type PurchaseReceiptSummary,
} from '@/api/purchase-receipts'
import { Empty } from '@/components/fields'
import type { PurchaseOrderItem } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

/**
 * 采购详情里的收货面板（T-51a）。
 *
 * 收货单是「采购订单 → 库存」之间唯一还缺的那张单：它确认之后，在途量才成立、
 * 入库单才追得回采购单。所以这里只做三件事——列出来、建一张、确认它。
 *
 * 在途量不在这里算：后端一次给回「每行已收量」，前端用订购减已收，
 * 口径只有一份；两边各算一次，迟早对不上。
 */
export function PurchaseReceiptPanel({
  orderId,
  items,
}: {
  orderId: number
  items: PurchaseOrderItem[]
}) {
  const [data, setData] = useState<PurchaseReceiptSummary>({ receipts: [], received_by_item: {} })
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [receivedDate, setReceivedDate] = useState<string>('')
  const [remark, setRemark] = useState('')
  // 每行本次收货数量：键是采购订单行 id。空/0 表示这一行这次不收。
  const [quantities, setQuantities] = useState<Record<number, number | null>>({})
  // 退货弹窗：按收货行填数量，累计不超收货量由后端校验。
  const [returning, setReturning] = useState<PurchaseReceipt | null>(null)
  const [returnQty, setReturnQty] = useState<Record<number, number | null>>({})
  const [returnReason, setReturnReason] = useState('')

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      setData(await purchaseReceiptApi.summary(orderId))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '收货记录加载失败')
    } finally {
      setLoading(false)
    }
  }, [orderId])

  useEffect(() => {
    void reload()
  }, [reload])

  const openCreate = () => {
    setQuantities({})
    setRemark('')
    setReceivedDate('')
    setOpen(true)
  }

  const submit = async () => {
    const rows = Object.entries(quantities)
      .filter(([, qty]) => (qty ?? 0) > 0)
      .map(([id, qty]) => ({ purchase_order_item_id: Number(id), qty: String(qty) }))
    if (rows.length === 0) {
      message.warning('至少填一行的收货数量')
      return
    }
    setSaving(true)
    try {
      await purchaseReceiptApi.create(orderId, {
        received_date: receivedDate,
        remark,
        items: rows,
      })
      message.success('收货单已建，确认后才会入库')
      setOpen(false)
      await reload()
    } catch (err) {
      // 超收是 409：后端会把「订购 / 已收 / 本次」三个数写进提示里，直接展示。
      message.error(err instanceof Error ? err.message : '建单失败')
    } finally {
      setSaving(false)
    }
  }

  const confirm = async (record: PurchaseReceipt) => {
    try {
      await purchaseReceiptApi.confirm(record.id)
      message.success('已确认，库存已入库')
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '确认失败')
    }
  }

  const submitReturn = async () => {
    if (!returning) return
    const rows = Object.entries(returnQty)
      .filter(([, qty]) => (qty ?? 0) > 0)
      .map(([id, qty]) => ({ purchase_receipt_item_id: Number(id), qty: String(qty) }))
    if (rows.length === 0) {
      message.warning('至少填一行的退货数量')
      return
    }
    try {
      const doc = await purchaseReceiptApi.returnItems(returning.id, {
        reason: returnReason,
        items: rows,
      })
      message.success(`已生成退货出库单 ${doc.doc_no}，库存已减少`)
      setReturning(null)
      setReturnQty({})
      setReturnReason('')
      await reload()
    } catch (err) {
      // 超退是 409，后端会把「收到 / 已退 / 本次」写进提示。
      message.error(err instanceof Error ? err.message : '退货失败')
    }
  }

  const remove = async (record: PurchaseReceipt) => {
    try {
      await purchaseReceiptApi.remove(record.id)
      message.success('收货单已删除')
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      title="收货"
      size="small"
      style={{ marginTop: 16 }}
      extra={
        <Button size="small" onClick={openCreate}>
          新建收货单
        </Button>
      }
    >
      <Table<PurchaseReceipt>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={data.receipts}
        pagination={false}
        locale={{ emptyText: '还没有收货记录。建单并确认后，库存才会计入。' }}
        columns={[
          { title: '单号', dataIndex: 'doc_no', width: 160 },
          {
            title: '到货日',
            dataIndex: 'received_date',
            width: 120,
            render: (v: string) => formatDate(v),
          },
          {
            title: '状态',
            dataIndex: 'status',
            width: 100,
            render: (v: number) =>
              v === 20 ? <Tag color="green">已确认</Tag> : <Tag color="default">草稿</Tag>,
          },
          {
            title: '收货数量',
            dataIndex: 'items',
            render: (_: unknown, record) => {
              const rows = record.items ?? []
              if (rows.length === 0) return <Empty />
              return (
                <Space size={12} wrap>
                  {rows.map((it) => (
                    <span key={it.id}>
                      {it.sku_code} × {formatQty(it.qty)} {it.unit}
                    </span>
                  ))}
                </Space>
              )
            },
          },
          {
            title: '操作',
            width: 150,
            render: (_: unknown, record) =>
              record.status === 10 ? (
                <Space size={4}>
                  <Popconfirm
                    title="确认这张收货单？"
                    description="确认后会在同一事务里生成并确认一张采购入库单，库存随之增加；之后不能再改。"
                    okText="确认收货"
                    cancelText="返回"
                    onConfirm={() => void confirm(record)}
                  >
                    <Button size="small" type="link">
                      确认
                    </Button>
                  </Popconfirm>
                  <Popconfirm
                    title="删除这张草稿？"
                    okText="删除"
                    cancelText="返回"
                    onConfirm={() => void remove(record)}
                  >
                    <Button size="small" type="link" danger>
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              ) : (
                <Space size={4}>
                  <Button
                    size="small"
                    type="link"
                    danger
                    onClick={() => {
                      setReturning(record)
                      setReturnQty({})
                      setReturnReason('')
                    }}
                  >
                    退货
                  </Button>
                  <span className="cell-sub">已入库</span>
                </Space>
              ),
          },
        ]}
      />

      <Modal
        open={open}
        title="新建收货单"
        width={760}
        onCancel={() => setOpen(false)}
        onOk={() => void submit()}
        confirmLoading={saving}
        okText="建单（草稿）"
        cancelText="取消"
        forceRender
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Space>
            <span>到货日</span>
            <DatePicker
              onChange={(_, dateString) => setReceivedDate(String(dateString))}
              placeholder="留空取今天"
            />
          </Space>

          <Table<PurchaseOrderItem>
            rowKey="id"
            size="small"
            dataSource={items}
            pagination={false}
            columns={[
              { title: 'SKU', dataIndex: 'sku_code', width: 140 },
              { title: '品名', dataIndex: 'name_cn' },
              {
                title: '订购',
                dataIndex: 'qty',
                width: 100,
                align: 'right' as const,
                render: (v: string) => formatQty(v),
              },
              {
                title: '已收（含草稿）',
                width: 130,
                align: 'right' as const,
                render: (_: unknown, record) => {
                  const got = data.received_by_item[String(record.id)]
                  return got ? formatQty(got) : '0'
                },
              },
              {
                title: '本次收货',
                width: 130,
                render: (_: unknown, record) => (
                  <InputNumber
                    min={0}
                    step={1}
                    style={{ width: '100%' }}
                    value={quantities[record.id] ?? null}
                    onChange={(v) => setQuantities((prev) => ({ ...prev, [record.id]: v }))}
                  />
                ),
              },
            ]}
          />

          <Input
            placeholder="备注（可选）"
            value={remark}
            onChange={(e) => setRemark(e.target.value)}
            maxLength={255}
          />
          <span className="cell-sub">
            建出来的是草稿，确认后才生成入库单；累计收货超过订购量会被拒绝。
          </span>
        </Space>
      </Modal>
      <Modal
        open={!!returning}
        title={returning ? `采购退货 · ${returning.doc_no}` : '采购退货'}
        width={720}
        onCancel={() => setReturning(null)}
        onOk={() => void submitReturn()}
        okText="确认退货"
        cancelText="取消"
        okButtonProps={{ danger: true }}
        forceRender
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <span className="cell-sub">
            退货会生成一张采购退货出库单，库存随之减少；累计退货不得超过收货量。
          </span>
          <Table<PurchaseReceiptItem>
            rowKey="id"
            size="small"
            dataSource={returning?.items ?? []}
            pagination={false}
            columns={[
              { title: 'SKU', dataIndex: 'sku_code', width: 140 },
              { title: '品名', dataIndex: 'name_cn' },
              {
                title: '收货量',
                dataIndex: 'qty',
                width: 110,
                align: 'right' as const,
                render: (v: string) => formatQty(v),
              },
              {
                title: '退货量',
                width: 120,
                render: (_: unknown, record) => (
                  <InputNumber
                    min={0}
                    style={{ width: '100%' }}
                    value={returnQty[record.id] ?? null}
                    onChange={(v) => setReturnQty((prev) => ({ ...prev, [record.id]: v }))}
                  />
                ),
              },
            ]}
          />
          <Input
            placeholder="退货原因（如：焊点开裂，工厂返工）"
            value={returnReason}
            onChange={(e) => setReturnReason(e.target.value)}
            maxLength={255}
          />
        </Space>
      </Modal>

    </Card>
  )
}
