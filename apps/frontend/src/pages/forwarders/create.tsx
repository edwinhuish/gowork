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
    <PageForm
      action="create"
      title="新建货代"
      // 「新建」的默认值放在这里，不写在共享的 ForwarderFormFields 上：
      // 那个组件编辑页也用，编辑时表单级 initialValues 里已经有记录的值，
      // Form.Item 再给 initialValue 就会撞车 —— antd 每加载一次刷一条
      // 「Form already set 'initialValues'」告警。记录值写在后面，
      // 有记录就以记录为准。
      formProps={{
        ...formProps,
        initialValues: { entity_type: 0, ...formProps.initialValues },
      }}
      saveButtonProps={saveButtonProps}
    >
      <ForwarderFormFields />
    </PageForm>
  )
}
