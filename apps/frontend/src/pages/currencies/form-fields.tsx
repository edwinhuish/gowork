import { Form, Input, InputNumber, Select, Switch } from 'antd'

import { CURRENCY_STATUS_OPTIONS } from '@/constants/catalog'

/**
 * 币种表单字段。
 *
 * 新建与编辑共用，唯一差别是编辑时代码只读：币种代码是业务身份
 * （单据上存的就是这个三位码），改它会让引用它的历史汇率与单据指向空虚。
 */
export function CurrencyFormFields({ codeReadOnly = false }: { codeReadOnly?: boolean }) {
  return (
    <>
      <Form.Item
        name="code"
        label="币种代码"
        rules={[
          { required: true, message: '请填写币种代码' },
          { pattern: /^[A-Za-z]{3}$/, message: '必须是 3 位字母，如 USD' },
        ]}
        extra="ISO 4217 三位字母。创建后不可修改"
      >
        <Input placeholder="USD" disabled={codeReadOnly} />
      </Form.Item>

      <Form.Item
        name="name"
        label="名称"
        rules={[
          { required: true, message: '请填写币种名称' },
          { max: 64, message: '最多 64 个字符' },
        ]}
      >
        <Input placeholder="美元" />
      </Form.Item>

      <Form.Item name="symbol" label="符号" rules={[{ max: 8, message: '最多 8 个字符' }]}>
        <Input placeholder="$" />
      </Form.Item>

      <Form.Item
        name="decimal_places"
        label="小数位"
        initialValue={2}
        extra="展示与舍入精度。人民币 2 位，日元 0 位"
      >
        <InputNumber style={{ width: '100%' }} min={0} max={6} precision={0} />
      </Form.Item>

      <Form.Item
        name="is_base"
        label="本币"
        valuePropName="checked"
        extra="全库只能有一个本币，设为本币会自动取消原来的那一个"
      >
        <Switch />
      </Form.Item>

      <Form.Item
        name="status"
        label="归档"
        initialValue={1}
        extra="归档后不再出现在下拉与缺汇率待办里；已有汇率的历史单据仍按它折算"
      >
        <Select options={CURRENCY_STATUS_OPTIONS} />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="选填" />
      </Form.Item>
    </>
  )
}
