import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Receivable } from '@/types'

export function ReceivableCreate() {
  const { formProps, saveButtonProps } = useForm<Receivable>({
    resource: 'receivables',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建应收" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <SettlementFormFields direction="receivable" />
      </Form>
    </Create>
  )
}
