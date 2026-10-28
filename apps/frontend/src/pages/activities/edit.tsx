import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Activity } from '@/types'

import { ActivityFormFields } from './form-fields'

export function ActivityEdit() {
  const { formProps, saveButtonProps } = useForm<Activity>({
    resource: 'activities',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit saveButtonProps={saveButtonProps} title="编辑跟进">
      <Form {...formProps} layout="vertical" className="entity-form">
        <ActivityFormFields partnerReadOnly />
      </Form>
    </Edit>
  )
}
