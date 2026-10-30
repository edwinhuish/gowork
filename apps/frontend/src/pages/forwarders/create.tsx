import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Partner } from '@/types'

import { ForwarderFormFields } from './form-fields'

export function ForwarderCreate() {
  const { formProps, saveButtonProps } = useForm<Partner>({
    resource: 'forwarders',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建货代" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ForwarderFormFields />
    </PageForm>
  )
}
