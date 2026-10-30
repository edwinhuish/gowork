import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Partner } from '@/types'

import { SupplierFormFields } from './form-fields'

export function SupplierCreate() {
  const { formProps, saveButtonProps } = useForm<Partner>({
    resource: 'suppliers',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建供应商" formProps={formProps} saveButtonProps={saveButtonProps}>
      <SupplierFormFields />
    </PageForm>
  )
}
