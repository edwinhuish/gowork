import { useForm } from '@refinedev/antd'
import { Divider } from 'antd'

import { PageForm } from '@/components/page'
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
    <PageForm
      action="create"
      title="新建出货单"
      formProps={{
        ...formProps,
        initialValues: { allocations: [] },
      }}
      saveButtonProps={saveButtonProps}
      wide
    >
      <ShipmentFormFields />

      <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
        出货明细
      </Divider>

      <AllocationEditor />
    </PageForm>
  )
}
