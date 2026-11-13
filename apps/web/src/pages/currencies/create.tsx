import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Currency } from '@/types'

import { CurrencyFormFields } from './form-fields'

export function CurrencyCreate() {
  const { formProps, saveButtonProps } = useForm<Currency>({
    resource: 'currencies',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm
      action="create"
      title="新建币种"
      // 「新建」的默认值放在这里，不写在共享的 CurrencyFormFields 上：
      // 那个组件编辑页也用，编辑时表单级 initialValues 里已经有记录的值，
      // Form.Item 再给 initialValue 就会撞车（小数位、归档两个字段每页
      // 各刷一条「Form already set 'initialValues'」）。记录值在后，
      // 有记录就以记录为准。
      formProps={{
        ...formProps,
        initialValues: { decimal_places: 2, status: 1, ...formProps.initialValues },
      }}
      saveButtonProps={saveButtonProps}
    >
      <CurrencyFormFields />
    </PageForm>
  )
}
