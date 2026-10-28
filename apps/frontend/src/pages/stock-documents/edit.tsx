import { Alert, Divider, Form } from 'antd'
import { Edit, useForm } from '@refinedev/antd'

import type { StockDocument } from '@/types'

import { StockDocFormFields } from './form-fields'
import { StockDocItemsEditor } from './items-editor'

/**
 * 编辑出入库单。只有草稿可改：已确认的单据已经产生了库存流水，
 * 改它就是改历史；要纠正请新建一张反向单。
 */
export function StockDocumentEdit() {
  const { formProps, saveButtonProps, form } = useForm<StockDocument>({
    resource: 'stock-documents',
    action: 'edit',
    redirect: 'show',
  })

  const status = Form.useWatch<number | undefined>('status', form)
  const type = Form.useWatch<number | undefined>('type', form)
  const locked = status !== undefined && status !== 10

  return (
    <Edit
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      title="编辑出入库单"
      goBack={null}
    >
      {locked && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="这张单据已确认，不能再改"
          description="流水已经落地。要纠正只能新建一张反向单，这样两次操作都留在账上。"
        />
      )}

      <Form {...formProps} layout="vertical" className="entity-form">
        <StockDocFormFields typeReadOnly />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          明细
        </Divider>

        <StockDocItemsEditor docType={type} />
      </Form>
    </Edit>
  )
}
