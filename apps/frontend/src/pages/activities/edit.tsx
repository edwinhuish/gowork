import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Activity } from '@/types'

import { ActivityFormFields } from './form-fields'

export function ActivityEdit() {
  const { formProps, saveButtonProps } = useForm<Activity>({
    resource: 'activities',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm action="edit" title="编辑跟进" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ActivityFormFields partnerReadOnly />
    </PageForm>
  )
}
