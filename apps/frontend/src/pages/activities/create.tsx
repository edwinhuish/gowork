import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Activity } from '@/types'

import { ActivityFormFields } from './form-fields'

export function ActivityCreate() {
  const { formProps, saveButtonProps } = useForm<Activity>({
    resource: 'activities',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="记录跟进" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ActivityFormFields />
    </PageForm>
  )
}
