import { useSelect } from '@refinedev/core'
import { Form, Input, Select } from 'antd'

import { ACTIVITY_TYPE_OPTIONS } from '@/constants/catalog'
import type { Contact, Partner } from '@/types'

/** 今天的 YYYY-MM-DD。跟进九成是"记今天发生的"，给个默认值少三次点击。 */
function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 跟进记录表单。
 *
 * 联系人必须跟着主体走：先选往来单位，联系人下拉才可用且只列这一家的人。
 * 后端也会校验归属，这里只是不让用户先撞一次 400。
 */
export function ActivityFormFields({ partnerReadOnly = false }: { partnerReadOnly?: boolean }) {
  const partnerId = Form.useWatch<number | undefined>('partner_id')

  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { options: contactOptions, query: contactQuery } = useSelect<Contact>({
    resource: 'contacts',
    optionLabel: (item) => (item.title ? `${item.name}（${item.title}）` : item.name),
    optionValue: 'id',
    pagination: { mode: 'off' },
    filters: partnerId ? [{ field: 'partner_id', operator: 'eq', value: partnerId }] : [],
  })

  return (
    <>
      <Form.Item
        name="partner_id"
        label="往来单位"
        rules={[{ required: true, message: '请选择往来单位' }]}
        extra={partnerReadOnly ? '跟进记录不能转移到另一个主体，需要时请新建' : undefined}
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
        name="contact_id"
        label="联系人"
        extra="选填。跟具体某个人谈的，比「跟客户谈的」有用得多"
      >
        <Select
          placeholder={partnerId ? '不指定' : '请先选择往来单位'}
          allowClear
          disabled={!partnerId}
          options={contactOptions}
          loading={contactQuery.isLoading}
        />
      </Form.Item>

      <Form.Item
        name="type"
        label="跟进方式"
        initialValue="CALL"
        rules={[{ required: true, message: '请选择跟进方式' }]}
      >
        <Select options={ACTIVITY_TYPE_OPTIONS} />
      </Form.Item>

      <Form.Item
        name="occurred_at"
        label="发生日期"
        initialValue={today()}
        rules={[
          { required: true, message: '请填写发生日期' },
          { pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' },
        ]}
        extra="补录时填实际那天，不能默认今天 —— 否则「最近跟进」会以为是今天"
      >
        <Input placeholder="2026-10-28" />
      </Form.Item>

      <Form.Item
        name="next_follow_at"
        label="下次跟进"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
        extra="留空表示不需要再跟；早于今天即算「已到期」"
      >
        <Input placeholder="2026-11-05" />
      </Form.Item>

      <Form.Item
        name="subject"
        label="主题"
        className="form-item-full"
        rules={[
          { required: true, message: '请填写主题' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="电话确认包装与交期" />
      </Form.Item>

      <Form.Item name="content" label="内容" className="form-item-full">
        <Input.TextArea rows={4} showCount maxLength={2000} placeholder="客户说了什么、答应什么、下一步做什么" />
      </Form.Item>
    </>
  )
}
