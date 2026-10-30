import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Opportunity } from '@/types'

import { OpportunityFormFields } from './form-fields'

export function OpportunityCreate() {
  const { formProps, saveButtonProps } = useForm<Opportunity>({
    resource: 'opportunities',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建商机" formProps={formProps} saveButtonProps={saveButtonProps}>
      <OpportunityFormFields />
    </PageForm>
  )
}
