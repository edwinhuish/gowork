import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Lead } from '@/types'

import { LeadFormFields } from './form-fields'

export function LeadCreate() {
  const { formProps, saveButtonProps } = useForm<Lead>({
    resource: 'leads',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm
      action="create"
      title="新建线索"
      // 「新建」的默认值放在这里，不写在共享的 LeadFormFields 上：
      // 那个组件编辑页也用，编辑时表单级 initialValues 里已经有记录的值，
      // Form.Item 再给 initialValue 就会撞车 —— antd 每次加载刷一条
      // 「Form already set 'initialValues' with path 'status'」。
      // 记录值在后，有记录就以记录为准。
      formProps={{
        ...formProps,
        initialValues: { status: 10, ...formProps.initialValues },
      }}
      saveButtonProps={saveButtonProps}
    >
      <LeadFormFields />
    </PageForm>
  )
}
