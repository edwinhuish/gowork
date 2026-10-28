import { Form, Input, Select } from 'antd'

import { LEAD_SOURCE_OPTIONS, LEAD_STATUS_OPTIONS } from '@/constants/catalog'

/**
 * 线索表单。
 *
 * 状态里刻意保留「已转化」但不建议手选：直接选它不会生成主体，
 * 后端会拒绝（转化必须走转化功能）。放在这里是为了让编辑页能正确显示当前值。
 */
export function LeadFormFields({ statusReadOnly = false }: { statusReadOnly?: boolean }) {
  return (
    <>
      <Form.Item
        name="company"
        label="公司 / 询盘主体"
        rules={[
          { required: true, message: '请填写公司名' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="Ferreteria El Sol" />
      </Form.Item>

      <Form.Item
        name="source"
        label="来源渠道"
        rules={[{ required: true, message: '请选择来源渠道' }]}
        extra="来源是投放决策的唯一依据，所以做成枚举而不是自由文本"
      >
        <Select options={LEAD_SOURCE_OPTIONS} placeholder="展会 / 阿里 / 搜索 / 推荐" />
      </Form.Item>

      <Form.Item name="country" label="国家/地区" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input placeholder="Ecuador" />
      </Form.Item>

      <Form.Item name="contact_name" label="联系人" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input placeholder="Carlos" />
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
        name="status"
        label="状态"
        initialValue={10}
        rules={[{ required: true, message: '请选择状态' }]}
        extra="要生成客户请用列表里的「转化」；这里直接选已转化会被拒绝"
      >
        <Select options={LEAD_STATUS_OPTIONS} disabled={statusReadOnly} />
      </Form.Item>

      <Form.Item name="demand" label="需求描述" className="form-item-full">
        <Input.TextArea rows={3} showCount maxLength={2000} placeholder="要什么规格、多少量、什么时候要" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="选填" />
      </Form.Item>
    </>
  )
}
