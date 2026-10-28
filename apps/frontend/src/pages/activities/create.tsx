import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Activity } from '@/types'

import { ActivityFormFields } from './form-fields'

export function ActivityCreate() {
  const { formProps, saveButtonProps } = useForm<Activity>({
    resource: 'activities',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="记录跟进" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <ActivityFormFields />
      </Form>
    </Create>
  )
}
