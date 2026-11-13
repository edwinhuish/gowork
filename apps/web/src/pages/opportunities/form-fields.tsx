import { useSelect } from '@refinedev/core'
import { Form, Input, InputNumber, Select } from 'antd'

import { OPPORTUNITY_STAGE_OPTIONS } from '@/constants/catalog'
import type { Currency, Partner } from '@/types'

/** 今天的 YYYY-MM-DD。 */
// 导出给新建页用：默认值放在新建页（见 create.tsx），编辑页不需要。
export function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 商机表单。
 *
 * 概率留空即按阶段取默认值（后端决定），所以这里不做"选阶段自动填概率"的联动：
 * 联动会让用户改过的概率被阶段覆盖，而销售对自己手里的单子有判断。
 */
export function OpportunityFormFields({ partnerReadOnly = false }: { partnerReadOnly?: boolean }) {
  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: 'partners',
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
        label="往来单位"
        rules={[{ required: true, message: '请选择往来单位' }]}
        extra={partnerReadOnly ? '商机不能转移到另一个主体，需要时请新建' : undefined}
      >
        <Select
          placeholder="请选择"
          showSearch
          disabled={partnerReadOnly}
          options={partnerOptions}
          loading={partnerQuery.isLoading}
          filterOption={(input, option) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
      </Form.Item>

      <Form.Item
        name="subject"
        label="商机名称"
        rules={[
          { required: true, message: '请填写商机名称' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="2026 秋季五金补货" />
      </Form.Item>

      <Form.Item
        name="stage"
        label="阶段"
        rules={[{ required: true, message: '请选择阶段' }]}
        extra="非终态之间可来回调整；赢单与输单是终态，之后不能再改"
      >
        <Select options={OPPORTUNITY_STAGE_OPTIONS} />
      </Form.Item>

      {/* stringMode：金额以字符串收发，走一遍 JS number 就等于把后端的定点精度丢了。 */}
      <Form.Item name="amount" label="预计金额">
        <InputNumber
          style={{ width: '100%' }}
          min={0}
          step={100}
          precision={2}
          stringMode
          placeholder="0.00"
        />
      </Form.Item>

      <Form.Item
        name="currency_code"
        label="币种"
        tooltip="留空按本币。非本币会按当日汇率折算本币金额"
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
        name="probability"
        label="赢单概率（%）"
        tooltip="留空按阶段取默认值；赢单恒 100、输单恒 0"
      >
        <InputNumber style={{ width: '100%' }} min={0} max={100} precision={0} placeholder="按阶段默认" />
      </Form.Item>

      <Form.Item
        name="expected_close_date"
        label="预计成交日"
        rules={[
          { required: true, message: '请填写预计成交日' },
          { pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' },
        ]}
      >
        <Input placeholder={today()} />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="选填" />
      </Form.Item>
    </>
  )
}
