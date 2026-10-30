import { useForm } from '@refinedev/antd'
import { Form, Input } from 'antd'

import { PageForm } from '@/components/page'
import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Receivable } from '@/types'

export function ReceivableEdit() {
  const { formProps, saveButtonProps, query } = useForm<Receivable>({
    resource: 'receivables',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm
      action="edit"
      title={`编辑应收${formProps.initialValues?.doc_no ? ` · ${formProps.initialValues.doc_no}` : ''}`}
      formProps={formProps}
      saveButtonProps={saveButtonProps}
      loading={query?.isLoading}
    >
      {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}
      <Form.Item name="updated_at" hidden>
        <Input />
      </Form.Item>
      <SettlementFormFields direction="receivable" editing />
    </PageForm>
  )
}
