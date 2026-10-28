import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import { SettlementFormFields } from '@/pages/settlements/form-fields'
import type { Payable } from '@/types'

export function PayableCreate() {
  const { formProps, saveButtonProps } = useForm<Payable>({
    resource: 'payables',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建应付" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <SettlementFormFields direction="payable" />
      </Form>
    </Create>
  )
}
