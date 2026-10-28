import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Currency } from '@/types'

import { CurrencyFormFields } from './form-fields'

export function CurrencyEdit() {
  const { formProps, saveButtonProps } = useForm<Currency>({
    resource: 'currencies',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit saveButtonProps={saveButtonProps} title="编辑币种">
      <Form {...formProps} layout="vertical" className="entity-form">
        <CurrencyFormFields codeReadOnly />
      </Form>
    </Edit>
  )
}
