import { useSelect } from '@refinedev/core'
import { Form, Input, Select } from 'antd'

import { TRADE_TERM_OPTIONS } from '@/constants/catalog'
import type { Currency, Partner } from '@/types'

import { todayString } from '../orders/form-fields'

/**
 * 采购订单表头字段。
 *
 * 供应商下拉取 /suppliers：采购的对象必须是供应商角色，后端也会校验。
 * 日期沿用订单页的文本框 + pattern，不额外引入日期库。
 */
export function PurchaseFormFields({ supplierReadOnly = false }: { supplierReadOnly?: boolean }) {
  const { options: supplierOptions, query: supplierQuery } = useSelect<Partner>({
    resource: 'suppliers',
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
        name="supplier_id"
        label="供应商"
        rules={[{ required: true, message: '请选择供应商' }]}
        extra={supplierReadOnly ? '采购单不能换供应商，需要给别家下单请新建一张' : undefined}
      >
        <Select
          placeholder="请选择"
          showSearch
          disabled={supplierReadOnly}
          options={supplierOptions}
          loading={supplierQuery.isLoading}
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
        label="采购币种"
        tooltip="与销售币种往往不同（收美元、付人民币）；按当日汇率折本币锁进单据"
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
        extra={`留空取今天（${todayString()}）`}
      >
        <Input placeholder={todayString()} />
      </Form.Item>

      <Form.Item
        name="delivery_date"
        label="承诺到货日"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
        extra="可留空；不能早于下单日"
      >
        <Input placeholder="2026-12-20" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="验货要求、包装、唛头等" />
      </Form.Item>
    </>
  )
}
