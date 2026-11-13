import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Activity } from '@/types'

import { ActivityFormFields, today } from './form-fields'

export function ActivityCreate() {
  const { formProps, saveButtonProps } = useForm<Activity>({
    resource: 'activities',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm
      action="create"
      title="记录跟进"
      // 同 LeadCreate：默认值属于「新建」，不写在共享的 ActivityFormFields 上
      // （编辑时与记录值撞车，antd 每页刷一条 initialValues 告警）。
      formProps={{
        ...formProps,
        initialValues: { type: 'CALL', occurred_at: today(), ...formProps.initialValues },
      }}
      saveButtonProps={saveButtonProps}
    >
      <ActivityFormFields />
    </PageForm>
  )
}
