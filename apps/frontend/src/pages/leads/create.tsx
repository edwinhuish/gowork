import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Lead } from '@/types'

import { LeadFormFields } from './form-fields'

export function LeadCreate() {
  const { formProps, saveButtonProps } = useForm<Lead>({
    resource: 'leads',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建线索" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <LeadFormFields />
      </Form>
    </Create>
  )
}
