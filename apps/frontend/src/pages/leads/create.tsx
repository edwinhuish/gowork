import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Lead } from '@/types'

import { LeadFormFields } from './form-fields'

export function LeadCreate() {
  const { formProps, saveButtonProps } = useForm<Lead>({
    resource: 'leads',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建线索" formProps={formProps} saveButtonProps={saveButtonProps}>
      <LeadFormFields />
    </PageForm>
  )
}
