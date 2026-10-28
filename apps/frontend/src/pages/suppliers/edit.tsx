import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Supplier } from '@/types'

import { SupplierFormFields } from './form-fields'

export function SupplierEdit() {
  const { formProps, saveButtonProps, query } = useForm<Supplier>({
    resource: 'suppliers',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑供应商${formProps.initialValues?.code ? ` · ${formProps.initialValues.code}` : ''}`}
    >
      <Form {...formProps} layout="vertical" className="entity-form">
        <SupplierFormFields />
      </Form>
    </Edit>
  )
}
