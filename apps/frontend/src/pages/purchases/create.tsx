import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { LineItemsEditor } from '@/components/line-items-editor'
import type { PurchaseOrder } from '@/types'

import { PurchaseFormFields } from './form-fields'
import { SalesOrderImport } from './sales-order-import'

export function PurchaseCreate() {
  const { formProps, saveButtonProps, form } = useForm<PurchaseOrder>({
    resource: 'purchase-orders',
    action: 'create',
    redirect: 'show',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建采购订单" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{ trade_term: 'FOB', items: [] }}
      >
        <PurchaseFormFields />

        <Divider orientation="left" style={{ margin: '8px 0 12px' }}>
          明细
        </Divider>

        <SalesOrderImport form={form} />

        <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
      </Form>
    </Create>
  )
}
