import { useSelect } from '@refinedev/core'
import { Form, Input, Select } from 'antd'

import { TRADE_TERM_OPTIONS } from '@/constants/catalog'
import type { Currency, Partner } from '@/types'

/** 默认有效期：报价单基本都是"一个月内有效"，给个默认值少两次点击。 */
export function defaultValidUntil(): string {
  const d = new Date()
  d.setDate(d.getDate() + 30)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 报价单表头字段。
 *
 * 日期用文本框而不是日期选择器：项目没有引入 dayjs 之外的日期依赖，
 * 而 antd 的 DatePicker 需要把值在 dayjs 与字符串之间来回转，多一处出错的地方。
 * 后端只认 YYYY-MM-DD，这里用 pattern 挡住明显的手误。
 */
export function QuotationFormFields({ partnerReadOnly = false }: { partnerReadOnly?: boolean }) {
  const { options: customerOptions, query: customerQuery } = useSelect<Partner>({
    resource: 'customers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { options: currencyOptions, query: currencyQuery } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'code',
    pagination: { mode: 'off' },
  })

  return (
    <>
      <Form.Item
        name="partner_id"
        label="客户"
        rules={[{ required: true, message: '请选择客户' }]}
        extra={partnerReadOnly ? '报价单不能改客户，需要报价给别家请新建一张' : undefined}
      >
        <Select
          placeholder="请选择"
          showSearch
          disabled={partnerReadOnly}
          options={customerOptions}
          loading={customerQuery.isLoading}
          filterOption={(input, option) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
      </Form.Item>

      <Form.Item
        name="trade_term"
        label="贸易术语"
        tooltip="留空取客户档案上的默认术语；客户也没设才需要手填。FOB 的海运费买方承担、CIF 卖方承担，同一批货两个报价值可以差 10%"
      >
        <Select options={TRADE_TERM_OPTIONS} allowClear placeholder="按客户默认" />
      </Form.Item>

      <Form.Item
        name="currency_code"
        label="报价币种"
        tooltip="留空取客户的默认结算币种，客户也没设才按本币。非本币按当日汇率折本币算毛利，汇率在保存时锁进单据"
      >
        <Select
          placeholder="按客户默认"
          allowClear
          showSearch
          options={currencyOptions}
          loading={currencyQuery.isLoading}
          filterOption={(input, option) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
      </Form.Item>

      <Form.Item
        name="valid_until"
        label="有效期截止日"
        rules={[
          { required: true, message: '请填写有效期' },
          { pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' },
        ]}
        extra="含当天。过期后不可确认、不可转订单"
      >
        <Input placeholder="2026-11-30" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="付款条件、包装要求等" />
      </Form.Item>
    </>
  )
}
