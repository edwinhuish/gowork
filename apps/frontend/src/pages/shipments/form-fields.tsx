import { Form, Input, Select } from 'antd'

import { CHARGE_ALLOC_BASIS_OPTIONS } from '@/constants/catalog'
import { todayString } from '../orders/form-fields'

/**
 * 出货单表头。
 *
 * 只有两个字段：出货日与备注。客户、币种、金额都不在这里——
 * 一票可以装多张订单的货（拼柜），把客户挂在票头上等于逼人拆柜。
 */
export function ShipmentFormFields() {
  return (
    <>
      <Form.Item
        name="ship_date"
        label="出货日"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
        extra="留空取今天；决定这票落在哪个统计月份"
      >
        <Input placeholder={todayString()} />
      </Form.Item>

      {/* 运杂费按票发生、按行归集（T-59）：拼柜时高价值小体积的货会背走多数运费，
          按体积或重量摊更贴近实际占用。默认按金额，与改造前一致。 */}
      <Form.Item
        name="charge_alloc_basis"
        label="运杂费分摊"
        tooltip="这票的运费按什么摊到各产品行。改它只影响本票的毛利下钻，不改票面费用总额"
      >
        <Select options={CHARGE_ALLOC_BASIS_OPTIONS} placeholder="按金额" allowClear />
      </Form.Item>

      <Form.Item name="remark" label="备注" className="form-item-full">
        <Input.TextArea
          rows={2}
          maxLength={255}
          showCount
          placeholder="柜号、唛头、装运港、货代联系方式等"
        />
      </Form.Item>
    </>
  )
}
