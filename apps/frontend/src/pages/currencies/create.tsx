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
    <PageForm action="create" title="新建币种" formProps={formProps} saveButtonProps={saveButtonProps}>
      <CurrencyFormFields />
    </PageForm>
  )
}
