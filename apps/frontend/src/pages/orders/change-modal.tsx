import { useState } from 'react'
import { Form, Input, InputNumber, Modal, Table, Typography, message } from 'antd'

import { salesOrderApi, type SalesOrderChangeInput } from '@/api/sales-orders'
import { Empty, Mono } from '@/components/fields'
import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { SalesOrder, SalesOrderItem } from '@/types'
import { formatQty } from '@/utils/format'

/** 表单里一行的新值，顺序与 order.items 一一对应。 */
interface LineDraft {
  qty: string
  unit_price: string
}

interface ChangeDraft {
  reason: string
  delivery_date?: string
  lines: LineDraft[]
}

/** 定点字符串比大小只为判断"这行有没有动"与做提示，不参与任何提交值。 */
function sameNumber(a?: string | null, b?: string | null) {
  return Number(a ?? 0) === Number(b ?? 0)
}

/**
 * 已确认订单的变更单录入弹窗。
 *
 * 为什么不是把编辑页解禁：已确认的订单是对客户的承诺，客户手里的 PDF、工厂照着做的
 * 采购单、已经出掉的货，都认当时那个数与那个交期。直接改等于让"订单上写着什么"
 * 随最后一个编辑它的人变化。所以这里只提供"改价 / 改量 / 改交期"，并强制填原因。
 *
 * 明细只能原地改，不能增删行：出货分配与以销定采都按行 id 引用。要加减货是另一笔生意。
 */
export function ChangeModal({
  open,
  order,
  onClose,
  onChanged,
}: {
  open: boolean
  order: SalesOrder
  onClose: () => void
  /** 提交成功后由调用方刷新订单详情与变更历史。 */
  onChanged: () => void | Promise<void>
}) {
  const [form] = Form.useForm<ChangeDraft>()
  const [submitting, setSubmitting] = useState(false)
  // 每次变更都是"一次新的操作"：键在用户意图这一层生成，失败时保持不变——
  // 失败后重试命中的是同一次意图，服务端若上次其实已经成功会直接回放。
  const { key: idempotencyKey, rotate } = useIdempotencyKey()

  const items = order.items ?? []
  const originDate = order.delivery_date ? order.delivery_date.slice(0, 10) : ''

  const submit = async () => {
    const values = await form.validateFields()
    const drafts = values.lines ?? []

    // 只把真变了的行发出去：没动的行后端也会跳过，但在前端就滤掉能让
    // "这次改了几行"与提交内容一致，出错时读日志不用再推一遍。
    const lines = items.reduce<SalesOrderChangeInput['lines']>((acc, item, index) => {
      const draft = drafts[index]
      if (!draft) return acc
      if (sameNumber(draft.qty, item.qty) && sameNumber(draft.unit_price, item.unit_price)) {
        return acc
      }
      acc.push({ sales_order_item_id: item.id, qty: draft.qty, unit_price: draft.unit_price })
      return acc
    }, [])

    const nextDate = (values.delivery_date ?? '').trim()
    if (lines.length === 0 && nextDate === originDate) {
      message.warning('这次变更没有任何改动')
      return
    }

    const body: SalesOrderChangeInput = {
      reason: values.reason.trim(),
      lines,
      updated_at: order.updated_at,
      // 交期没变就不带这个字段（后端把"省略"读作不动它）；
      // 清空交期必须带空串过去，否则两种意思分不开。
      ...(nextDate === originDate ? {} : { delivery_date: nextDate }),
    }

    setSubmitting(true)
    try {
      await salesOrderApi.change(order.id, body, idempotencyKey)
      message.success('变更已记录，订单已更新')
      rotate()
      form.resetFields()
      onClose()
      await onChanged()
    } catch (err) {
      // 409 在这里是常态：别人先改过、或改后数量低于已出货量，后端都解释得很清楚。
      message.error(err instanceof Error ? err.message : '变更提交失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      open={open}
      title={`变更订单 ${order.doc_no}`}
      width={960}
      okText="提交变更"
      cancelText="返回"
      confirmLoading={submitting}
      onOk={() => void submit()}
      onCancel={onClose}
      // 关掉即卸载，下次打开重新按当前值预填；否则会留着上一次的输入。
      destroyOnHidden
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          reason: '',
          delivery_date: originDate,
          lines: items.map((item) => ({ qty: item.qty, unit_price: item.unit_price })),
        }}
      >
        <Form.Item
          name="reason"
          label="变更原因"
          rules={[{ required: true, whitespace: true, message: '请填写变更原因' }]}
          extra="改完这张单，事后只有这一句话能说明当时为什么改"
        >
          <Input.TextArea rows={2} maxLength={255} showCount placeholder="如：客户要求降价 3%，8 月 20 日电话确认" />
        </Form.Item>

        <Form.Item
          name="delivery_date"
          label="约定交期"
          rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
          extra="留空表示取消交期；不改就保持原样，只改明细时这一项不用动"
        >
          <Input placeholder={originDate || '2026-12-31'} />
        </Form.Item>

        <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
          数量不能低于已出货量（发走的货不会因为改单回到仓库）；不能增删明细行，
          要加减货请作废重下。金额由后端按新值重算，这里只显示原值。
        </Typography.Paragraph>

        <Table
          rowKey="id"
          size="small"
          dataSource={items}
          pagination={false}
          scroll={{ x: 860 }}
          locale={{ emptyText: '这张订单没有明细' }}
        >
          <Table.Column
            dataIndex="sku_code"
            title="SKU"
            width={130}
            render={(value: string) => <Mono>{value}</Mono>}
          />
          <Table.Column
            dataIndex="name_cn"
            title="品名"
            render={(value: string) => value || <Empty />}
          />
          <Table.Column
            dataIndex="shipped_qty"
            title="已出"
            width={80}
            align="right"
            render={(value?: string) => (value === undefined ? <Empty /> : formatQty(value))}
          />
          <Table.Column
            dataIndex="qty"
            title="原数量"
            width={90}
            align="right"
            render={(value: string) => formatQty(value)}
          />
          <Table.Column
            title="新数量"
            width={130}
            render={(_, record: SalesOrderItem, index: number) => (
              <Form.Item
                name={['lines', index, 'qty']}
                style={{ marginBottom: 0 }}
                rules={[
                  { required: true, message: '必填' },
                  {
                    validator: (_, value) => {
                      if (value && Number(value) < Number(record?.shipped_qty ?? 0)) {
                        return Promise.reject(
                          new Error(`已出 ${formatQty(record?.shipped_qty)}，不能改到更低`),
                        )
                      }
                      return Promise.resolve()
                    },
                  },
                ]}
              >
                <InputNumber
                  style={{ width: '100%' }}
                  min={0.001}
                  step={1}
                  precision={3}
                  stringMode
                />
              </Form.Item>
            )}
          />
          <Table.Column
            dataIndex="unit_price"
            title="原单价"
            width={100}
            align="right"
            render={(value: string) => Number(value).toFixed(2)}
          />
          <Table.Column
            title="新单价"
            width={130}
            render={(_, _record: SalesOrderItem, index: number) => (
              <Form.Item name={['lines', index, 'unit_price']} style={{ marginBottom: 0 }}>
                <InputNumber
                  style={{ width: '100%' }}
                  min={0}
                  step={0.01}
                  precision={4}
                  stringMode
                />
              </Form.Item>
            )}
          />
        </Table>
      </Form>
    </Modal>
  )
}
