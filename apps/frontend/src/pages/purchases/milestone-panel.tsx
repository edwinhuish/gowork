import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Card, Form, Input, Modal, Space, Table, Tag, Tooltip, message } from 'antd'

import { purchaseOrderApi } from '@/api/purchase-orders'
import { Mono } from '@/components/fields'
import { PURCHASE_NODES, type PurchaseMilestone, type PurchaseOrder } from '@/types'

const NODE_LABEL = new Map<number, string>(PURCHASE_NODES.map((n) => [n.value, n.label]))

/** 计划日已过且没有完成日 = 逾期。与后端同一口径，只用于上色，不做统计。 */
function isOverdue(m: PurchaseMilestone | undefined): boolean {
  if (!m?.plan_date || m.done_date) return false
  return m.plan_date < new Date().toISOString().slice(0, 10)
}

/**
 * 采购跟单节点面板（T-38）。
 *
 * 行是采购明细、列是五个节点：跟单的人拿着这张表就能回答
 * "这个 SKU 的货在工厂还是已经进仓、验货过没过"。
 *
 * 一格只存两件事：计划日与完成日。日期都填了就画绿勾，计划日过了还没完成的画红。
 */
export function PurchaseMilestonePanel({ order }: { order: PurchaseOrder }) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<{ itemId: number; node: number } | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<{ plan_date: string; done_date: string; remark: string }>()

  const { data, isLoading } = useQuery({
    queryKey: ['purchase-milestones', order.id],
    queryFn: () => purchaseOrderApi.milestones(order.id),
  })

  // (行, 节点) → 节点，表格按它取数。
  const byKey = useMemo(() => {
    const map = new Map<string, PurchaseMilestone>()
    for (const m of data ?? []) {
      map.set(`${m.purchase_order_item_id}-${m.node}`, m)
    }
    return map
  }, [data])

  const openEditor = (itemId: number, node: number) => {
    const existing = byKey.get(`${itemId}-${node}`)
    form.setFieldsValue({
      plan_date: existing?.plan_date ?? '',
      done_date: existing?.done_date ?? '',
      remark: existing?.remark ?? '',
    })
    setEditing({ itemId, node })
  }

  const save = async () => {
    if (!editing) return
    const values = await form.validateFields()
    setSaving(true)
    try {
      await purchaseOrderApi.saveMilestones(order.id, [
        {
          purchase_order_item_id: editing.itemId,
          node: editing.node,
          plan_date: values.plan_date?.trim() ?? '',
          done_date: values.done_date?.trim() ?? '',
          remark: values.remark?.trim() ?? '',
        },
      ])
      message.success('已保存')
      setEditing(null)
      await queryClient.invalidateQueries({ queryKey: ['purchase-milestones', order.id] })
    } catch (err) {
      // 出运早于下单这类错录由后端拒（400），文案直接用它的。
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const items = order.items ?? []

  return (
    <Card
      title="跟单节点"
      size="small"
      style={{ marginTop: 16 }}
      loading={isLoading}
      extra={<span className="cell-sub">点格子补录计划日与实际完成日</span>}
    >
      <Table
        rowKey="id"
        size="small"
        dataSource={items}
        pagination={false}
        scroll={{ x: 900 }}
        locale={{ emptyText: '这张采购单还没有明细' }}
      >
        <Table.Column
          title="物料"
          width={220}
          render={(_, row: { id: number; sku_code: string; name_cn: string }) => (
            <Space size={4}>
              <Mono>{row.sku_code}</Mono>
              <span className="cell-sub">{row.name_cn}</span>
            </Space>
          )}
        />
        {PURCHASE_NODES.map((node) => (
          <Table.Column
            key={node.value}
            title={node.label}
            width={130}
            render={(_, row: { id: number }) => {
              const m = byKey.get(`${row.id}-${node.value}`)
              const overdue = isOverdue(m)
              return (
                <Tooltip
                  title={
                    m?.operator
                      ? `${m.operator} 于 ${m.done_date} 补录${m.remark ? `：${m.remark}` : ''}`
                      : '点击补录'
                  }
                >
                  <Button
                    type="text"
                    size="small"
                    style={{ padding: 0, height: 'auto', textAlign: 'left' }}
                    onClick={() => openEditor(row.id, node.value)}
                  >
                    {!m?.plan_date && !m?.done_date ? (
                      <span className="cell-sub">未安排</span>
                    ) : (
                      <Space direction="vertical" size={0}>
                        <span className="cell-sub">计划 {m?.plan_date ?? '—'}</span>
                        {m?.done_date ? (
                          <Tag color="green" style={{ marginInlineEnd: 0 }}>
                            {m.done_date}
                          </Tag>
                        ) : overdue ? (
                          <Tag color="red" style={{ marginInlineEnd: 0 }}>
                            逾期
                          </Tag>
                        ) : (
                          <span className="cell-sub">未完成</span>
                        )}
                      </Space>
                    )}
                  </Button>
                </Tooltip>
              )
            }}
          />
        ))}
      </Table>

      <Modal
        open={!!editing}
        title={editing ? `补录 · ${NODE_LABEL.get(editing.node) ?? ''}` : '补录'}
        onCancel={() => setEditing(null)}
        onOk={() => void save()}
        confirmLoading={saving}
        okText="保存"
        cancelText="返回"
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="plan_date"
            label="计划完成日"
            rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
            extra="留空表示还没排计划；排了计划但过了日子没完成，列表上会标成逾期"
          >
            <Input placeholder="2026-11-05" />
          </Form.Item>
          <Form.Item
            name="done_date"
            label="实际完成日"
            rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
            extra="填了就算完成，并记下是谁补的；清空表示这件又变成没做完"
          >
            <Input placeholder="2026-11-03" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input placeholder="如：验货发现 3 箱破损，工厂返工" maxLength={255} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
