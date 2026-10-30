import { useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { PageForm } from '@/components/page'
import { LineItemsEditor } from '@/components/line-items-editor'
import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { SalesOrder } from '@/types'

import { CreditHint } from './credit-hint'
import { OrderFormFields } from './form-fields'

export function OrderCreate() {
  // 同一个幂等键覆盖"点一次、超时、再点一次"：服务端只会建一条订单。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps, form } = useForm<SalesOrder>({
    resource: 'sales-orders',
    action: 'create',
    redirect: 'show',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
  })

  return (
    <PageForm
      action="create"
      title="新建销售订单"
      formProps={{
        ...formProps,
        initialValues: {
        trade_term: 'FOB',
        items: [{ qty: '1', unit_price: '0' }],
        },
      }}
      saveButtonProps={saveButtonProps}
      wide
    >
      <OrderFormFields />

      {/* 选中客户就把额度摊开：超额不会在这里拦，但可以先看见差多少。 */}
      <CreditHint partnerId={Form.useWatch<number | undefined>('partner_id', form)} />

      <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
        明细
      </Divider>

      <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
    </PageForm>
  )
}
