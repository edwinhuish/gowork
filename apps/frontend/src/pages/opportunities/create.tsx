import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Opportunity } from '@/types'

import { OpportunityFormFields } from './form-fields'

export function OpportunityCreate() {
  const { formProps, saveButtonProps } = useForm<Opportunity>({
    resource: 'opportunities',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建商机" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <OpportunityFormFields />
      </Form>
    </Create>
  )
}
