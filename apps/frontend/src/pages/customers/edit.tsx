import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Customer } from '@/types'

import { CustomerFormFields } from './form-fields'

export function CustomerEdit() {
  const { formProps, saveButtonProps, query } = useForm<Customer>({
    resource: 'customers',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑客户${formProps.initialValues?.code ? ` · ${formProps.initialValues.code}` : ''}`}
    >
      <Form {...formProps} layout="vertical" className="entity-form">
        <CustomerFormFields />
      </Form>
    </Edit>
  )
}
