import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Currency } from '@/types'

import { CurrencyFormFields } from './form-fields'

export function CurrencyCreate() {
  const { formProps, saveButtonProps } = useForm<Currency>({
    resource: 'currencies',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建币种" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <CurrencyFormFields />
      </Form>
    </Create>
  )
}
