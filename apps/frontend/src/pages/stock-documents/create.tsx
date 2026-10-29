import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { StockDocument } from '@/types'

import { StockDocFormFields } from './form-fields'
import { StockDocItemsEditor } from './items-editor'

export function StockDocumentCreate() {
  // 单据重复提交会多出一张入库/出库单，确认后还要红字冲销——用幂等键挡住。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps, form } = useForm<StockDocument>({
    resource: 'stock-documents',
    action: 'create',
    redirect: 'show',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
  })

  const type = Form.useWatch<number | undefined>('type', form)

  return (
    <Create saveButtonProps={saveButtonProps} title="新建出入库单" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{ type: 10, items: [{ qty: '1' }] }}
      >
        <StockDocFormFields />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          明细
        </Divider>

        <StockDocItemsEditor docType={type} />
      </Form>
    </Create>
  )
}
