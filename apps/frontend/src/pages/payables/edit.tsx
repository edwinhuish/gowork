import { useForm } from '@refinedev/antd'
import { Form, Input } from 'antd'

import { PageForm } from '@/components/page'
import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Payable } from '@/types'

export function PayableEdit() {
  const { formProps, saveButtonProps, query } = useForm<Payable>({
    resource: 'payables',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm
      action="edit"
      title={`编辑应付${formProps.initialValues?.doc_no ? ` · ${formProps.initialValues.doc_no}` : ''}`}
      formProps={formProps}
      saveButtonProps={saveButtonProps}
      loading={query?.isLoading}
    >
      {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}
      <Form.Item name="updated_at" hidden>
        <Input />
      </Form.Item>
      <SettlementFormFields direction="payable" editing />
    </PageForm>
  )
}
