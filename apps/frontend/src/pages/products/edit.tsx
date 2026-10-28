import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Product } from '@/types'

import { ProductFormFields } from './form-fields'

export function ProductEdit() {
  const { formProps, saveButtonProps, query } = useForm<Product>({
    resource: 'products',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑产品${formProps.initialValues?.sku_code ? ` · ${formProps.initialValues.sku_code}` : ''}`}
    >
      <Form {...formProps} layout="vertical" className="entity-form">
        <ProductFormFields />
      </Form>
    </Edit>
  )
}
