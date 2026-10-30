import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Partner } from '@/types'

import { CustomerFormFields } from './form-fields'

export function CustomerCreate() {
  const { formProps, saveButtonProps } = useForm<Partner>({
    resource: 'customers',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建客户" formProps={formProps} saveButtonProps={saveButtonProps}>
      <CustomerFormFields />
    </PageForm>
  )
}
