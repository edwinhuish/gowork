import { useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { PageForm } from '@/components/page'
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
    <PageForm
      action="create"
      title="新建采购订单"
      formProps={{
        ...formProps,
        initialValues: { trade_term: 'FOB', items: [] },
      }}
      saveButtonProps={saveButtonProps}
      wide
    >
      <PurchaseFormFields />

      <Divider orientation="left" style={{ margin: '8px 0 12px' }}>
        明细
      </Divider>

      <SalesOrderImport form={form} />

      <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
    </PageForm>
  )
}
