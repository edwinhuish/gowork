import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Opportunity } from '@/types'

import { OpportunityFormFields } from './form-fields'

export function OpportunityEdit() {
  const { formProps, saveButtonProps } = useForm<Opportunity>({
    resource: 'opportunities',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit saveButtonProps={saveButtonProps} title="编辑商机">
      <Form {...formProps} layout="vertical" className="entity-form">
        <OpportunityFormFields partnerReadOnly />
      </Form>
    </Edit>
  )
}
