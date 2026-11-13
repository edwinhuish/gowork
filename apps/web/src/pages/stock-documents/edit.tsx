import { Alert, Divider, Form, Input } from 'antd'
import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
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
    <PageForm
      action="edit"
      title="编辑出入库单"
      formProps={formProps}
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      notice={
        locked ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="这张单据已确认，不能再改"
            description="流水已经落地。要纠正只能新建一张反向单，这样两次操作都留在账上。"
          />
        ) : null
      }
      wide
    >
      {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。
          必须是注册过的字段——antd 只提交已注册的项，光靠 initialValues
          带着 updated_at 是不够的，它会在提交时被丢掉。 */}
      <Form.Item name="updated_at" hidden>
        <Input />
      </Form.Item>
      <StockDocFormFields typeReadOnly />

      <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
        明细
      </Divider>

      <StockDocItemsEditor docType={type} />
    </PageForm>
  )
}
