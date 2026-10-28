import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Payable } from '@/types'

export function PayableEdit() {
  const { formProps, saveButtonProps, query } = useForm<Payable>({
    resource: 'payables',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑应付${formProps.initialValues?.doc_no ? ` · ${formProps.initialValues.doc_no}` : ''}`}
    >
      <Form {...formProps} layout="vertical" className="entity-form">
        <SettlementFormFields direction="payable" editing />
      </Form>
    </Edit>
  )
}
