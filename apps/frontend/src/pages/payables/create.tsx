import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
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
    <PageForm action="create" title="新建应付" formProps={formProps} saveButtonProps={saveButtonProps}>
      <SettlementFormFields direction="payable" />
    </PageForm>
  )
}
