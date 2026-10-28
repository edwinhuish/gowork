import { useSelect } from '@refinedev/core'
import { Form, Input, InputNumber, Select } from 'antd'

export type SettlementDirection = 'receivable' | 'payable'

/** 订单下拉只需要这几列，销售单与采购单字段名一致。 */
interface OrderOption {
  id: number
  doc_no: string
  currency_code: string
  /** 后端 money.Decimal：字符串，只用于下拉里的显示。 */
  amount_total: string
}

/**
 * 应收 / 应付的表单字段。两个模块共用，唯一差别是来源单据是谁。
 *
 * 表单里没有币种、汇率、对方主体：三个值都由后端从来源订单带出并锁定。
 * 让人在这里填汇率，就等于允许同一天为同一张订单填出两个本币金额。
 */
export function SettlementFormFields({
  direction,
  editing,
}: {
  direction: SettlementDirection
  editing?: boolean
}) {
  const isReceivable = direction === 'receivable'
  const { options, query } = useSelect<OrderOption>({
    resource: isReceivable ? 'sales-orders' : 'purchase-orders',
    optionLabel: (item) => `${item.doc_no} · ${item.currency_code} ${item.amount_total}`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  return (
    <>
      <Form.Item
        name="source_id"
        label={isReceivable ? '来源销售订单' : '来源采购订单'}
        rules={[{ required: true, message: '请选择来源单据' }]}
        tooltip={
          editing
            ? '来源不能改：改了等于这张单描述的是另一笔生意'
            : '只有已确认及之后的单据能开票，草稿与已取消的会被后端拒绝'
        }
      >
        <Select
          showSearch
          optionFilterProp="label"
          disabled={editing}
          loading={query.isLoading}
          placeholder={isReceivable ? '选择要开票的销售订单' : '选择要开票的采购订单'}
          options={options}
        />
      </Form.Item>

      <Form.Item
        name="amount"
        label="金额（原币）"
        tooltip="留空表示把来源单的未开票余额全开出来。超额会被后端拒绝——来源总额是硬上限"
        rules={[{ type: 'number', min: 0, message: '不能为负' }]}
      >
        <InputNumber style={{ width: '100%' }} min={0} step={0.01} precision={4} placeholder="留空 = 全额" />
      </Form.Item>

      <Form.Item
        name="due_date"
        label="到期日"
        tooltip="留空按对方账期推算（订单日 + 账期天数）；账期 0 表示即期，到期日就是订单日"
        rules={[
          {
            validator: (_, value?: string) => {
              const v = value?.trim()
              if (!v || /^\d{4}-\d{2}-\d{2}$/.test(v)) return Promise.resolve()
              return Promise.reject(new Error('格式应为 YYYY-MM-DD'))
            },
          },
        ]}
      >
        <Input placeholder="YYYY-MM-DD（留空按账期）" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full" rules={[{ max: 255, message: '最多 255 个字符' }]}>
        <Input placeholder="付款条件、开票说明，选填" maxLength={255} />
      </Form.Item>
    </>
  )
}
