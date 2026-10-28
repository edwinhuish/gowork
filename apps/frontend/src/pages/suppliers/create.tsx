import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Supplier } from '@/types'

import { SupplierFormFields } from './form-fields'

export function SupplierCreate() {
  const { formProps, saveButtonProps } = useForm<Supplier>({
    resource: 'suppliers',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建供应商" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <SupplierFormFields />
      </Form>
    </Create>
  )
}
