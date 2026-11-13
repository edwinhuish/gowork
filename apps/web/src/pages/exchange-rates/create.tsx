import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { ExchangeRate } from '@/types'

import { ExchangeRateFormFields } from './form-fields'

export function ExchangeRateCreate() {
  const { formProps, saveButtonProps } = useForm<ExchangeRate>({
    resource: 'exchange-rates',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="登记汇率" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ExchangeRateFormFields />
    </PageForm>
  )
}
