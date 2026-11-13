import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Receivable } from '@/types'

export function ReceivableCreate() {
  // 应收重复开票会让同一笔钱出现两张单，核销时对不上。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps } = useForm<Receivable>({
    resource: 'receivables',
    action: 'create',
    redirect: 'list',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
  })

  return (
    <PageForm action="create" title="新建应收" formProps={formProps} saveButtonProps={saveButtonProps}>
      <SettlementFormFields direction="receivable" />
    </PageForm>
  )
}
