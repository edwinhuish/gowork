import { useSelect } from '@refinedev/core'
import { Form, Input, Select, Switch } from 'antd'

import type { Partner } from '@/types'

/**
 * 联系人表单字段。
 *
 * 主体下拉取 /partners 而不是 /customers：联系人可能挂在客户、供应商，
 * 也可能挂在客供两用的主体上，按角色切开反而找不到人。
 */
export function ContactFormFields({ partnerReadOnly = false }: { partnerReadOnly?: boolean }) {
  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  return (
    <>
      <Form.Item
        name="partner_id"
        label="所属往来单位"
        rules={[{ required: true, message: '请选择往来单位' }]}
        extra={partnerReadOnly ? '联系人不能转移到另一个主体，需要时请新建' : undefined}
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
        name="name"
        label="姓名"
        rules={[
          { required: true, message: '请填写姓名' },
          { max: 64, message: '最多 64 个字符' },
        ]}
      >
        <Input placeholder="Carlos" />
      </Form.Item>

      <Form.Item name="title" label="职位" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input placeholder="采购经理 / 老板 / 跟单" />
      </Form.Item>

      <Form.Item name="phone" label="电话" rules={[{ max: 32, message: '最多 32 个字符' }]}>
        <Input placeholder="+593 9 0000 0001" />
      </Form.Item>

      <Form.Item
        name="email"
        label="邮箱"
        rules={[
          { type: 'email', message: '邮箱格式不正确' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input />
      </Form.Item>

      <Form.Item
        name="language"
        label="语言"
        rules={[{ max: 32, message: '最多 32 个字符' }]}
        extra="决定谁去跟：西语客户配西语业务，转述会丢信息"
      >
        <Input placeholder="西班牙语" />
      </Form.Item>

      <Form.Item
        name="channel"
        label="偏好渠道"
        rules={[{ max: 32, message: '最多 32 个字符' }]}
        extra="发错渠道等于没联系"
      >
        <Select
          placeholder="未指定"
          allowClear
          options={[
            { value: 'WhatsApp', label: 'WhatsApp' },
            { value: '微信', label: '微信' },
            { value: '邮件', label: '邮件' },
            { value: '电话', label: '电话' },
          ]}
        />
      </Form.Item>

      <Form.Item name="is_primary" label="主联系人" valuePropName="checked">
        <Switch />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="选填" />
      </Form.Item>
    </>
  )
}
