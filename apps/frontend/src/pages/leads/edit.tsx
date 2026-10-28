import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Lead } from '@/types'

import { LeadFormFields } from './form-fields'

export function LeadEdit() {
  const { formProps, saveButtonProps } = useForm<Lead>({
    resource: 'leads',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit saveButtonProps={saveButtonProps} title="编辑线索">
      <Form {...formProps} layout="vertical" className="entity-form">
        <LeadFormFields />
      </Form>
    </Edit>
  )
}
