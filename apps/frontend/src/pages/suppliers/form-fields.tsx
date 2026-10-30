import { Form, Input, Radio } from 'antd'

import { PartnerDuplicateHint } from '@/components/partner-duplicate-hint'
import { PARTNER_ENTITY_TYPE_OPTIONS } from '@/constants/catalog'

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

      {/* 国家/地区必须在这里也能改：客户与供应商是同一行记录，
          供应商页面不提交它的话，从供应商侧保存一次就会把国家清空
          （PUT 是全量覆盖，未提交的字符串字段按空值写入）。 */}
      <Form.Item name="country" label="国家/地区" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input placeholder="中国" />
      </Form.Item>

      <Form.Item
        name="tax_id"
        label="税号"
        tooltip="采购合同与发票用。工厂多为个人或个体户时可以留空"
        rules={[{ max: 32, message: '最多 32 个字符' }]}
      >
        <Input placeholder="统一社会信用代码 / VAT" />
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

      {/* 客户与供应商共用一行记录，重复的供应商同样该提示：同一家工厂两个编码，
          采购价就永远对不齐。 */}
      <PartnerDuplicateHint />

      <Form.Item name="address" label="地址" className="form-item-full" rules={[{ max: 255, message: '最多 255 个字符' }]}>
        <Input />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full" rules={[{ max: 255, message: '最多 255 个字符' }]}>
        <Input.TextArea rows={2} showCount maxLength={255} />
      </Form.Item>
    </>
  )
}
