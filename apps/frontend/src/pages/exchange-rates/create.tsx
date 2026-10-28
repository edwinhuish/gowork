import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { ExchangeRate } from '@/types'

import { ExchangeRateFormFields } from './form-fields'

export function ExchangeRateCreate() {
  const { formProps, saveButtonProps } = useForm<ExchangeRate>({
    resource: 'exchange-rates',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="登记汇率" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <ExchangeRateFormFields />
      </Form>
    </Create>
  )
}
