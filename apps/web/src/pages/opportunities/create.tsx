import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Opportunity } from '@/types'

import { OpportunityFormFields, today } from './form-fields'

export function OpportunityCreate() {
  const { formProps, saveButtonProps } = useForm<Opportunity>({
    resource: 'opportunities',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm
      action="create"
      title="新建商机"
      // 同 LeadCreate：默认值属于「新建」，不写在共享的 OpportunityFormFields 上
      // （编辑时与记录值撞车，antd 每页刷一条 initialValues 告警）。
      formProps={{
        ...formProps,
        initialValues: {
          stage: 10,
          amount: '0',
          expected_close_date: today(),
          ...formProps.initialValues,
        },
      }}
      saveButtonProps={saveButtonProps}
    >
      <OpportunityFormFields />
    </PageForm>
  )
}
