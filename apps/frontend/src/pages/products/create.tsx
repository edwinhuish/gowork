import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Product } from '@/types'

import { ProductFormFields } from './form-fields'

export function ProductCreate() {
  const { formProps, saveButtonProps } = useForm<Product>({
    resource: 'products',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建产品" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ProductFormFields />
    </PageForm>
  )
}
