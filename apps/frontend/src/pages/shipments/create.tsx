import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { Shipment } from '@/types'

import { AllocationEditor } from './allocation-editor'
import { ShipmentFormFields } from './form-fields'

export function ShipmentCreate() {
  // 重复提交会多一票货，占掉销售订单的未出数量——必须幂等。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps } = useForm<Shipment>({
    resource: 'shipments',
    action: 'create',
    redirect: 'show',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
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
