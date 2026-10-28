import { useSelect } from '@refinedev/core'
import { Divider, Form, Input, InputNumber, Select } from 'antd'

import { CATEGORY_OPTIONS, ENABLE_STATUS_OPTIONS } from '@/constants/catalog'

/**
 * 产品表单字段。
 *
 * 新建与编辑共用这一份：字段、校验规则、占位文案只写一次。
 * 改造前两个入口各自维护一份 11 字段的表单，加字段时漏改一边是必然的。
 *
 * 只用 <Form.Item>，不包含 <Form> 本身 —— 外壳（Create / Edit）各自决定
 * 提交行为，字段定义保持无状态。
 */
export function ProductFormFields() {
  // pagination mode off 会一次取满（dataProvider 里翻译成 page_size=200），
  // 否则供应商超过默认页长时下拉会静默丢项。v5 的 useSelect 直接返回 options，
  // 不再包一层 selectProps，所以要自己接 loading 和搜索过滤。
  const { options: supplierOptions, query: supplierQuery } = useSelect({
    resource: 'suppliers',
    optionLabel: 'name',
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  return (
    <>
      <Form.Item
        name="sku_code"
        label="SKU 编码"
        rules={[
          { required: true, message: '请填写 SKU 编码' },
          { max: 32, message: '最多 32 个字符' },
        ]}
        extra="格式：类别码(2) + 子类(2) - 流水(4)，如 BH01-0023"
      >
        <Input placeholder="BH01-0023" />
      </Form.Item>

      <Form.Item
        name="name_cn"
        label="中文品名"
        rules={[
          { required: true, message: '请填写中文品名' },
          { max: 128, message: '最多 128 个字符' },
        ]}
      >
        <Input placeholder="不锈钢合页" />
      </Form.Item>

      <Form.Item name="name_en" label="英文品名" rules={[{ max: 128, message: '最多 128 个字符' }]}>
        <Input placeholder="Stainless Steel Hinge" />
      </Form.Item>

      <Form.Item name="category" label="类别">
        <Select placeholder="未分类" allowClear options={CATEGORY_OPTIONS} />
      </Form.Item>

      <Form.Item name="spec" label="规格">
        <Input placeholder="100×75×3 mm" />
      </Form.Item>

      <Form.Item name="unit" label="单位">
        <Input placeholder="PCS" />
      </Form.Item>

      <Form.Item name="supplier_id" label="供应商">
        <Select
          placeholder="未指定"
          allowClear
          showSearch
          options={supplierOptions}
          loading={supplierQuery.isLoading}
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

      {/* stringMode：金额按字符串收发表单，走一遍 JS number 等于把后端的定点精度丢了。 */}
      <Form.Item name="cost_price" label="成本价" initialValue="0">
        <InputNumber
          style={{ width: '100%' }}
          min={0}
          step={0.01}
          precision={2}
          stringMode
          placeholder="0.00"
        />
      </Form.Item>

      <Form.Item name="sale_price" label="售价" initialValue="0">
        <InputNumber
          style={{ width: '100%' }}
          min={0}
          step={0.01}
          precision={2}
          stringMode
          placeholder="0.00"
        />
      </Form.Item>

      {/* 报关要素全是选填：它们只是默认值，真正申报以出货批次上填的为准。
          做成必填会逼人在打样阶段就编一个 HS 编码，那比留空更糟。 */}
      <Divider orientation="left" className="form-item-full" style={{ margin: '8px 0 16px' }}>
        报关要素（选填）
      </Divider>

      <Form.Item
        name="hs_code"
        label="HS 编码"
        rules={[{ max: 16, message: '最多 16 个字符' }]}
        extra="默认值；同一产品出口不同国家可能归不同税号"
      >
        <Input placeholder="7326909000" />
      </Form.Item>

      <Form.Item name="legal_unit" label="法定单位" rules={[{ max: 16, message: '最多 16 个字符' }]}>
        <Input placeholder="千克" />
      </Form.Item>

      <Form.Item
        name="declared_name_cn"
        label="申报品名（中文）"
        rules={[{ max: 128, message: '最多 128 个字符' }]}
      >
        <Input placeholder="不锈钢合页" />
      </Form.Item>

      <Form.Item
        name="declared_name_en"
        label="申报品名（英文）"
        rules={[{ max: 128, message: '最多 128 个字符' }]}
      >
        <Input placeholder="Stainless Steel Hinge" />
      </Form.Item>

      <Form.Item name="origin_country" label="原产地" rules={[{ max: 64, message: '最多 64 个字符' }]}>
        <Input placeholder="中国" />
      </Form.Item>

      <Form.Item
        name="material"
        label="材质与用途"
        rules={[{ max: 255, message: '最多 255 个字符' }]}
      >
        <Input placeholder="不锈钢，建筑用" />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea rows={2} maxLength={255} showCount placeholder="选填" />
      </Form.Item>
    </>
  )
}
