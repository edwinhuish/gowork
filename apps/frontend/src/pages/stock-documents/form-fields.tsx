import { useSelect } from '@refinedev/core'
import { Form, Input, Select } from 'antd'

import { STOCK_DOC_COUNT, STOCK_DOC_TYPE_OPTIONS } from '@/constants/catalog'
import { todayString } from '@/pages/orders/form-fields'
import type { Partner } from '@/types'

/**
 * 出入库单表头。
 *
 * 往来单位随类型切换：入库挂供应商、出库挂客户、盘点不需要。
 * 切换类型时会把已选的往来单位清掉——供应商 id 挂到出库单上，
 * 会在按客户看流水时冒出一个供应商。
 */
export function StockDocFormFields({ typeReadOnly = false }: { typeReadOnly?: boolean }) {
  const form = Form.useFormInstance()
  const type = Form.useWatch<number | undefined>('type', form) ?? 10
  const isCount = type === STOCK_DOC_COUNT
  // 入库找供应商，出库找客户；盘点不显示但 hook 不能条件调用。
  const partnerResource = type === 20 ? 'customers' : 'suppliers'

  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: partnerResource,
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
    queryOptions: { enabled: !isCount },
  })

  return (
    <>
      <Form.Item
        name="type"
        label="单据类型"
        rules={[{ required: true, message: '请选择类型' }]}
        extra={typeReadOnly ? '类型决定单号前缀，建好之后不能改' : undefined}
      >
        <Select
          options={STOCK_DOC_TYPE_OPTIONS}
          disabled={typeReadOnly}
          onChange={() => {
            // 换了类型，原来选的往来单位就不属于这一类了。
            form.setFieldValue('partner_id', undefined)
          }}
        />
      </Form.Item>

      <Form.Item
        name="doc_date"
        label="单据日期"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
        extra="留空取今天；决定这批货算在哪个月"
      >
        <Input placeholder={todayString()} />
      </Form.Item>

      {!isCount && (
        <Form.Item name="partner_id" label={type === 20 ? '客户' : '供应商'}>
          <Select
            placeholder="可留空"
            allowClear
            showSearch
            options={partnerOptions}
            loading={partnerQuery.isLoading}
            filterOption={(input, option) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
          />
        </Form.Item>
      )}

      <Form.Item
        name="source_doc_no"
        label="来源单号"
        rules={[{ max: 32, message: '最多 32 个字符' }]}
        extra="采购订单号 / 出货单号，只作追溯，不校验是否存在"
      >
        <Input placeholder={type === 20 ? 'DN-2610-001' : 'PO-2610-001'} />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="验货结论、货位、异常说明等" />
      </Form.Item>
    </>
  )
}
