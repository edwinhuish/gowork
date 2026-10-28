import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Product } from '@/types'

import { ProductFormFields } from './form-fields'

export function ProductCreate() {
  const { formProps, saveButtonProps } = useForm<Product>({
    resource: 'products',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建产品" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <ProductFormFields />
      </Form>
    </Create>
  )
}
