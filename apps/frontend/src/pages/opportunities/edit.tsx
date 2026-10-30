import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Opportunity } from '@/types'

import { OpportunityFormFields } from './form-fields'

export function OpportunityEdit() {
  const { formProps, saveButtonProps } = useForm<Opportunity>({
    resource: 'opportunities',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm action="edit" title="编辑商机" formProps={formProps} saveButtonProps={saveButtonProps}>
      <OpportunityFormFields partnerReadOnly />
    </PageForm>
  )
}
