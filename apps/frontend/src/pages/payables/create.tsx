import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Payable } from '@/types'

export function PayableCreate() {
  // 应付重复开单会让同一笔采购款挂两张单。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps } = useForm<Payable>({
    resource: 'payables',
    action: 'create',
    redirect: 'list',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建应付" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <SettlementFormFields direction="payable" />
      </Form>
    </Create>
  )
}
