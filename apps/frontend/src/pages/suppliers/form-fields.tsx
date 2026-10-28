import { Form, Input, Radio, Select } from 'antd'

import { ENABLE_STATUS_OPTIONS, PARTNER_ENTITY_TYPE_OPTIONS } from '@/constants/catalog'

/** 供应商表单字段。新建与编辑共用，校验规则只此一份。 */
export function SupplierFormFields() {
  return (
    <>
      <Form.Item
        name="code"
        label="供应商编码"
        rules={[
          { required: true, message: '请填写编码' },
          { max: 32, message: '最多 32 个字符' },
        ]}
      >
        <Input placeholder="SUP001" />
      </Form.Item>

      <Form.Item
        name="name"
        label="名称"
        rules={[
          { required: true, message: '请填写名称' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="1688 店铺或工厂名" />
      </Form.Item>

      <Form.Item
        name="entity_type"
        label="主体类型"
        initialValue={0}
        tooltip="1688 上的个人店铺、个体户按个人登记，报关与开票资料与公司不同"
      >
        <Radio.Group options={PARTNER_ENTITY_TYPE_OPTIONS} optionType="button" buttonStyle="solid" />
      </Form.Item>

      <Form.Item name="contact" label="联系人" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input />
      </Form.Item>

      <Form.Item
        name="phone"
        label="电话"
        rules={[{ max: 32, message: '最多 32 个字符' }]}
      >
        <Input />
      </Form.Item>

      <Form.Item
        name="email"
        label="邮箱"
        rules={[{ type: 'email', message: '邮箱格式不正确' }, { max: 128, message: '最多 128 个字符' }]}
      >
        <Input />
      </Form.Item>

      <Form.Item name="status" label="状态" initialValue={1}>
        <Select options={ENABLE_STATUS_OPTIONS} />
      </Form.Item>

      <Form.Item name="address" label="地址" className="form-item-full" rules={[{ max: 255, message: '最多 255 个字符' }]}>
        <Input />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full" rules={[{ max: 255, message: '最多 255 个字符' }]}>
        <Input.TextArea rows={2} showCount maxLength={255} />
      </Form.Item>
    </>
  )
}
