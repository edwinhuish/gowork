import { useSelect } from '@refinedev/core'
import { Form, Input, InputNumber, Radio, Select } from 'antd'

import { PartnerDuplicateHint } from '@/components/partner-duplicate-hint'
import {
  ENABLE_STATUS_OPTIONS,
  PARTNER_ENTITY_TYPE_OPTIONS,
  PARTNER_GRADE_OPTIONS,
  SETTLEMENT_METHOD_OPTIONS,
  TRADE_TERM_OPTIONS,
} from '@/constants/catalog'
import type { Currency } from '@/types'

/** 客户表单字段。新建与编辑共用。 */
export function CustomerFormFields() {
  // 默认结算币种从这里选。off 模式一次取满，币种只有几个，不必分页。
  const { options: currencyOptions, query: currencyQuery } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'code',
    pagination: { mode: 'off' },
  })

  return (
    <>
      <Form.Item
        name="code"
        label="客户编码"
        rules={[
          { required: true, message: '请填写编码' },
          { max: 32, message: '最多 32 个字符' },
        ]}
      >
        <Input placeholder="CUS001" />
      </Form.Item>

      <Form.Item
        name="name"
        label="名称"
        rules={[
          { required: true, message: '请填写名称' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="进口商或终端门店" />
      </Form.Item>

      <Form.Item
        name="entity_type"
        label="主体类型"
        initialValue={0}
        tooltip="个人主体的报关资料与发票抬头和公司不同，需要单独标记"
      >
        <Radio.Group options={PARTNER_ENTITY_TYPE_OPTIONS} optionType="button" buttonStyle="solid" />
      </Form.Item>

      <Form.Item name="country" label="国家/地区" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input placeholder="厄瓜多尔" />
      </Form.Item>

      <Form.Item
        name="tax_id"
        label="税号"
        tooltip="开票与报关用。公司主体不强制，但单证上缺它容易被客户或海关退回"
        rules={[{ max: 32, message: '最多 32 个字符' }]}
      >
        <Input placeholder="RUC / VAT / EIN" />
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

      {/* 放在四个参与查重的字段之后：名称、国家、电话、邮箱都填了才谈得上比对。 */}
      <PartnerDuplicateHint />

      {/* 三个分组维度放在商务条件之前：它们是「这家客户是谁」的一部分，
          而贸易术语与账期是「怎么跟他做生意」。 */}
      <Form.Item
        name="customer_group"
        label="客户分组"
        tooltip="报表分组用，自由填写。不建字典表：分组会随市场调整，只有十来种且只有老板会改"
        rules={[{ max: 64, message: '最多 64 个字符' }]}
      >
        <Input placeholder="南美批发商" />
      </Form.Item>

      <Form.Item
        name="region"
        label="销售区域"
        tooltip="我方划分的片区，不是国家——同一个国家可以归两个片区"
        rules={[{ max: 64, message: '最多 64 个字符' }]}
      >
        <Input placeholder="南美一区" />
      </Form.Item>

      <Form.Item
        name="grade"
        label="客户分级"
        tooltip="只做标签与筛选，不影响权限与折扣；留空表示还没分级"
      >
        <Select placeholder="未分级" allowClear options={PARTNER_GRADE_OPTIONS} />
      </Form.Item>

      <Form.Item
        name="trade_term"
        label="贸易术语"
        tooltip="决定运费与保险由谁承担，直接进报价的构成"
      >
        <Select placeholder="未指定" allowClear options={TRADE_TERM_OPTIONS} />
      </Form.Item>

      <Form.Item name="settlement_method" label="结算方式" tooltip="决定账期与风险，也决定报价的价格基础">
        <Select placeholder="未指定" allowClear options={SETTLEMENT_METHOD_OPTIONS} />
      </Form.Item>

      <Form.Item
        name="credit_days"
        label="账期（天）"
        initialValue={0}
        tooltip="从出货或开票起算。0 表示款到发货"
      >
        <InputNumber style={{ width: '100%' }} min={0} max={365} precision={0} />
      </Form.Item>

      <Form.Item
        name="credit_limit"
        label="信用额度（本币）"
        initialValue={0}
        tooltip="0 表示不限，而不是一分不给"
      >
        <InputNumber
          style={{ width: '100%' }}
          min={0}
          step={1000}
          precision={2}
          stringMode
          placeholder="0.00"
        />
      </Form.Item>

      <Form.Item name="currency_code" label="默认结算币种" tooltip="留空表示按报价单，再退到本币">
        <Select
          placeholder="跟随报价单 / 本币"
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
