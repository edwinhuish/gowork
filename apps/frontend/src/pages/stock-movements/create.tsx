import { useForm } from '@refinedev/antd'
import { Form, Input, InputNumber, Radio, Select, Typography } from 'antd'

import { PageForm } from '@/components/page'
import { ProductSelect } from '@/components/product-select'
import { MANUAL_STOCK_MOVEMENT_OPTIONS, STOCK_DIRECTION_OPTIONS } from '@/constants/catalog'
import { todayString } from '@/pages/orders/form-fields'
import type { StockMovement } from '@/types'

/**
 * 手工录入库存流水。
 *
 * 只给期初 / 盘点盈亏 / 其它三类：采购入库与销售出库必须挂在单据上，
 * 后端会拒（400）。这一页的定位是"把系统上线时的存量与盘点差异补进来"，
 * 不是绕过单据改库存的后门。
 */
export function StockMovementCreate() {
  const { formProps, saveButtonProps } = useForm<StockMovement>({
    resource: 'stock-movements',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm
      action="create"
      title="手工录入库存流水"
      formProps={{ ...formProps, initialValues: { type: 10, direction: 1 } }}
      saveButtonProps={saveButtonProps}
      notice={
        <Typography.Paragraph type="secondary">
          每一次库存变化都要有一条流水。录错了就删掉那条（库存会跟着回滚），不要直接调数。
        </Typography.Paragraph>
      }
    >
        <Form.Item
          name="product_id"
          label="产品"
          rules={[{ required: true, message: '请选择产品' }]}
        >
          <ProductSelect placeholder="搜 SKU / 品名 / 客户编号" />
        </Form.Item>

        <Form.Item
          name="type"
          label="类型"
          rules={[{ required: true, message: '请选择类型' }]}
          extra="采购入库 / 销售出库由单据产生，不在这里录"
        >
          <Select options={MANUAL_STOCK_MOVEMENT_OPTIONS} />
        </Form.Item>

        <Form.Item
          name="direction"
          label="方向"
          rules={[{ required: true, message: '请选择方向' }]}
        >
          <Radio.Group options={STOCK_DIRECTION_OPTIONS} optionType="button" />
        </Form.Item>

        <Form.Item
          name="qty"
          label="数量"
          rules={[{ required: true, message: '填数量' }]}
          extra="始终填正数，出库靠上面的方向表示"
        >
          <InputNumber
            style={{ width: '100%' }}
            min={0.001}
            step={1}
            precision={3}
            stringMode
            placeholder="0"
          />
        </Form.Item>

        <Form.Item
          name="occurred_at"
          label="发生日"
          rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
          extra="留空取今天；这是业务发生的那一天，不是录入时间"
        >
          <Input placeholder={todayString()} />
        </Form.Item>

        <Form.Item name="remark" label="备注" className="form-item-full">
          <Input.TextArea rows={2} maxLength={255} showCount placeholder="盘点表编号、样品用途等" />
        </Form.Item>
    </PageForm>
  )
}
