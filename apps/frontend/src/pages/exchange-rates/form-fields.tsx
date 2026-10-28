import { useSelect } from '@refinedev/core'
import { Form, Input, InputNumber, Select } from 'antd'

import type { Currency } from '@/types'

/** 今天的 YYYY-MM-DD。录入汇率九成是"录今天的牌价"，给个默认值少两次点击。 */
function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 汇率表单字段。
 *
 * 只有新建，没有编辑：改一条历史汇率会让当时所有单据的成本按新价重算，
 * 那笔生意到底按什么价成交就再也说不清。录错就删掉重录。
 */
export function ExchangeRateFormFields() {
  // 泛型写上 Currency，optionLabel 的回调才知道 item 有 code / name；
  // 不写时 refine 只能推断出 { id: any }，读 item.code 会报 TS2339。
  const { options: currencyOptions, query: currencyQuery } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  return (
    <>
      <Form.Item
        name="currency_code"
        label="外币"
        rules={[{ required: true, message: '请选择外币' }]}
        extra="本币是折算基准，不需要登记汇率"
      >
        <Select
          placeholder="请选择"
          showSearch
          options={currencyOptions}
          loading={currencyQuery.isLoading}
          filterOption={(input, option) =>
            String(option?.label ?? '').toLowerCase().includes(input.toLowerCase())
          }
        />
      </Form.Item>

      <Form.Item
        name="rate"
        label="汇率"
        rules={[{ required: true, message: '请填写汇率' }]}
        extra="含义固定：1 单位外币 = 多少本币"
      >
        <InputNumber
          style={{ width: '100%' }}
          min={0}
          step={0.0001}
          precision={6}
          placeholder="7.100000"
        />
      </Form.Item>

      <Form.Item
        name="effective_date"
        label="生效日期"
        initialValue={today()}
        rules={[
          { required: true, message: '请填写生效日期' },
          { pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' },
        ]}
        extra="同一天同一币种只能有一条；之后没有新价的日子沿用最近一条"
      >
        <Input placeholder="2026-10-28" />
      </Form.Item>

      <Form.Item name="source" label="来源" rules={[{ max: 32, message: '最多 32 个字符' }]}>
        <Input placeholder="中国银行牌价 / 手工" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="选填" />
      </Form.Item>
    </>
  )
}
