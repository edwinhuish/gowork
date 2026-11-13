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
    <PageForm
      action="create"
      title="新建产品"
      // 「新建」的默认值放在这里，不写在共享的 ProductFormFields 上：
      // 那个组件编辑页也用，编辑时表单级 initialValues 里已经有记录的值，
      // Form.Item 再给 initialValue 就会撞车 —— antd 每加载一次刷一条
      // 「Form already set 'initialValues'」告警。记录值写在后面，
      // 有记录就以记录为准。
      formProps={{
        ...formProps,
        initialValues: { cost_price: '0', sale_price: '0', ...formProps.initialValues },
      }}
      saveButtonProps={saveButtonProps}
    >
      <ProductFormFields />
    </PageForm>
  )
}
