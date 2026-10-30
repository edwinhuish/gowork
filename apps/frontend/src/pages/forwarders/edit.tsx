import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Partner } from '@/types'

import { ForwarderFormFields } from './form-fields'

export function ForwarderEdit() {
  const { formProps, saveButtonProps, query } = useForm<Partner>({
    resource: 'forwarders',
    action: 'edit',
    redirect: 'list',
  })

  // 单号只用于标题，读一次就够：formProps.initialValues 在数据到达后才填上。
  const code = formProps.initialValues?.code as string | undefined

  return (
    <PageForm
      action="edit"
      title={`编辑货代${code ? ` · ${code}` : ''}`}
      formProps={formProps}
      saveButtonProps={saveButtonProps}
      notice={query?.isLoading ? null : undefined}
    >
      <ForwarderFormFields />
    </PageForm>
  )
}
