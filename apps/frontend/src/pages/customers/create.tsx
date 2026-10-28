import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Party } from '@/types'

import { CustomerFormFields } from './form-fields'

export function CustomerCreate() {
  const { formProps, saveButtonProps } = useForm<Party>({
    resource: 'customers',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建客户" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <CustomerFormFields />
      </Form>
    </Create>
  )
}
