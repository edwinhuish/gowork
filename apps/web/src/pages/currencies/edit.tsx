import { useForm } from '@refinedev/antd'
import { Form, Input } from 'antd'

import { PageForm } from '@/components/page'
import type { Currency } from '@/types'

import { CurrencyFormFields } from './form-fields'

export function CurrencyEdit() {
  const { formProps, saveButtonProps } = useForm<Currency>({
    resource: 'currencies',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm action="edit" title="编辑币种" formProps={formProps} saveButtonProps={saveButtonProps}>
      {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}
      <Form.Item name="updated_at" hidden>
        <Input />
      </Form.Item>
      <CurrencyFormFields codeReadOnly />
    </PageForm>
  )
}
