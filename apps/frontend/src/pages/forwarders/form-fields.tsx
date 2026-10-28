import { Form, Input, Radio, Select } from 'antd'

import { ENABLE_STATUS_OPTIONS, PARTNER_ENTITY_TYPE_OPTIONS } from '@/constants/catalog'

/**
 * 货代表单字段。新建与编辑共用，校验规则只此一份。
 *
 * 比供应商少一组商务条件（贸易术语、账期、信用额度）：货代不进采购与报价，
 * 那些字段对它没有意义。`md_partner` 上那几列对货代保持为空即可 ——
 * 共用一张表的代价就是这里要主动不展示，而不是靠另一张表天然隔离。
 */
export function ForwarderFormFields() {
  return (
    <>
      <Form.Item
        name="code"
        label="货代编码"
        rules={[
          { required: true, message: '请填写编码' },
          { max: 32, message: '最多 32 个字符' },
        ]}
      >
        <Input placeholder="FWD001" />
      </Form.Item>

      <Form.Item
        name="name"
        label="名称"
        rules={[
          { required: true, message: '请填写名称' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="货代或报关行名称" />
      </Form.Item>

      <Form.Item
        name="entity_type"
        label="主体类型"
        initialValue={0}
        tooltip="报关行多以公司名义接单；个人代理按个人登记，报关资料与发票抬头不同"
      >
        <Radio.Group options={PARTNER_ENTITY_TYPE_OPTIONS} optionType="button" buttonStyle="solid" />
      </Form.Item>

      <Form.Item name="contact" label="联系人" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input />
      </Form.Item>

      <Form.Item name="phone" label="电话" rules={[{ max: 32, message: '最多 32 个字符' }]}>
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

      <Form.Item
        name="address"
        label="地址"
        className="form-item-full"
        rules={[{ max: 255, message: '最多 255 个字符' }]}
      >
        <Input />
      </Form.Item>

      <Form.Item
        name="remark"
        label="备注"
        className="form-item-full"
        rules={[{ max: 255, message: '最多 255 个字符' }]}
      >
        <Input.TextArea rows={2} showCount maxLength={255} placeholder="常用航线、订舱窗口等" />
      </Form.Item>
    </>
  )
}
