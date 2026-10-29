import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { LineItemsEditor } from '@/components/line-items-editor'
import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { PurchaseOrder } from '@/types'

import { PurchaseFormFields } from './form-fields'
import { SalesOrderImport } from './sales-order-import'

export function PurchaseCreate() {
  // 采购单重复提交会多出一张单，还得手工作废——用幂等键挡住。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps, form } = useForm<PurchaseOrder>({
    resource: 'purchase-orders',
    action: 'create',
    redirect: 'show',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
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
