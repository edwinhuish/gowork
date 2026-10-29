import { Edit, useForm } from '@refinedev/antd'
import { Form, Input } from 'antd'

import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Receivable } from '@/types'

export function ReceivableEdit() {
  const { formProps, saveButtonProps, query } = useForm<Receivable>({
    resource: 'receivables',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑应收${formProps.initialValues?.doc_no ? ` · ${formProps.initialValues.doc_no}` : ''}`}
    >
      <Form {...formProps} layout="vertical" className="entity-form">
        {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}
        <Form.Item name="updated_at" hidden>
          <Input />
        </Form.Item>
        <SettlementFormFields direction="receivable" editing />
      </Form>
    </Edit>
  )
}
