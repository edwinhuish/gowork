import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Partner } from '@/types'

import { ForwarderFormFields } from './form-fields'

export function ForwarderEdit() {
  const { formProps, saveButtonProps, query } = useForm<Partner>({
    resource: 'forwarders',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑货代${formProps.initialValues?.code ? ` · ${formProps.initialValues.code}` : ''}`}
    >
      <Form {...formProps} layout="vertical" className="entity-form">
        <ForwarderFormFields />
      </Form>
    </Edit>
  )
}
