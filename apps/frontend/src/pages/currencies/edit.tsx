import { Edit, useForm } from '@refinedev/antd'
import { Form, Input } from 'antd'

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
        {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}
        <Form.Item name="updated_at" hidden>
          <Input />
        </Form.Item>
        <CurrencyFormFields codeReadOnly />
      </Form>
    </Edit>
  )
}
