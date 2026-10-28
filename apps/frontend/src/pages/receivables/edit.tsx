import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

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
        <SettlementFormFields direction="receivable" editing />
      </Form>
    </Edit>
  )
}
