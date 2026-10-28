import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Partner } from '@/types'

import { ForwarderFormFields } from './form-fields'

export function ForwarderCreate() {
  const { formProps, saveButtonProps } = useForm<Partner>({
    resource: 'forwarders',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建货代" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <ForwarderFormFields />
      </Form>
    </Create>
  )
}
