import { useSelect } from '@refinedev/core'
import { Form, Input, Select } from 'antd'

import { TRADE_TERM_OPTIONS } from '@/constants/catalog'
import type { Currency, Partner } from '@/types'

export function todayString(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 销售订单表头字段。
 *
 * 日期同样是文本框 + pattern：项目没有单独的日期库依赖，
 * DatePicker 要在 dayjs 与字符串之间来回转，多一处出错的地方而收益很小。
 */
export function OrderFormFields({ partnerReadOnly = false }: { partnerReadOnly?: boolean }) {
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
        extra={partnerReadOnly ? '订单不能改客户，需要给别家下单请新建一张' : undefined}
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
        rules={[{ required: true, message: '请选择贸易术语' }]}
      >
        <Select options={TRADE_TERM_OPTIONS} />
      </Form.Item>

      <Form.Item
        name="currency_code"
        label="成交币种"
        tooltip="留空按本币。非本币按当日汇率折本币，汇率在保存时锁进单据"
      >
        <Select
          placeholder="本币"
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
        name="order_date"
        label="下单日"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
        extra="留空取今天；决定这单落在哪个统计月份"
      >
        <Input placeholder={todayString()} />
      </Form.Item>

      <Form.Item
        name="delivery_date"
        label="约定交期"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
        extra="可留空；不能早于下单日"
      >
        <Input placeholder="2026-12-31" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="包装、唛头、付款条件等" />
      </Form.Item>
    </>
  )
}
