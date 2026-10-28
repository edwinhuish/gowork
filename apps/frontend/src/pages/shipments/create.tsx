import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import type { Shipment } from '@/types'

import { AllocationEditor } from './allocation-editor'
import { ShipmentFormFields } from './form-fields'

export function ShipmentCreate() {
  const { formProps, saveButtonProps } = useForm<Shipment>({
    resource: 'shipments',
    action: 'create',
    redirect: 'show',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建出货单" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{ allocations: [] }}
      >
        <ShipmentFormFields />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          出货明细
        </Divider>

        <AllocationEditor />
      </Form>
    </Create>
  )
}
