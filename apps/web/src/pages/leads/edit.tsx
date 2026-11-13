import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Lead } from '@/types'

import { LeadFormFields } from './form-fields'

export function LeadEdit() {
  const { formProps, saveButtonProps } = useForm<Lead>({
    resource: 'leads',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm action="edit" title="编辑线索" formProps={formProps} saveButtonProps={saveButtonProps}>
      <LeadFormFields />
    </PageForm>
  )
}
