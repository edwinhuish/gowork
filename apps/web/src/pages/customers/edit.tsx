import { useForm } from '@refinedev/antd'
import { Form, Input } from 'antd'

import { PageForm } from '@/components/page'
import type { Partner } from '@/types'

import { CustomerFormFields } from './form-fields'

export function CustomerEdit() {
  const { formProps, saveButtonProps, query } = useForm<Partner>({
    resource: 'customers',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm
      action="edit"
      title={`编辑客户${formProps.initialValues?.code ? ` · ${formProps.initialValues.code}` : ''}`}
      formProps={formProps}
      saveButtonProps={saveButtonProps}
      loading={query?.isLoading}
    >
      {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。
          必须是注册过的字段——antd 只提交已注册的项，光靠 initialValues
          带着 updated_at 是不够的，它会在提交时被丢掉。 */}
      <Form.Item name="updated_at" hidden>
        <Input />
      </Form.Item>
      <CustomerFormFields />
    </PageForm>
  )
}
