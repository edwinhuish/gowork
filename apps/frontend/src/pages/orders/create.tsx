import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { LineItemsEditor } from '@/components/line-items-editor'
import type { SalesOrder } from '@/types'

import { OrderFormFields } from './form-fields'

export function OrderCreate() {
  const { formProps, saveButtonProps, form } = useForm<SalesOrder>({
    resource: 'sales-orders',
    action: 'create',
    redirect: 'show',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建销售订单" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{
          trade_term: 'FOB',
          items: [{ qty: 1, unit_price: 0 }],
        }}
      >
        <OrderFormFields />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          明细
        </Divider>

        <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
      </Form>
    </Create>
  )
}
