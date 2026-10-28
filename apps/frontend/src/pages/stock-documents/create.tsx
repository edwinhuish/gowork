import { Create, useForm } from '@refinedev/antd'
import { Divider, Form } from 'antd'

import type { StockDocument } from '@/types'

import { StockDocFormFields } from './form-fields'
import { StockDocItemsEditor } from './items-editor'

export function StockDocumentCreate() {
  const { formProps, saveButtonProps, form } = useForm<StockDocument>({
    resource: 'stock-documents',
    action: 'create',
    redirect: 'show',
  })

  const type = Form.useWatch<number | undefined>('type', form)

  return (
    <Create saveButtonProps={saveButtonProps} title="新建出入库单" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{ type: 10, items: [{ qty: 1 }] }}
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
