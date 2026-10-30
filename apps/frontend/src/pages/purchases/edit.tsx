import { useForm } from '@refinedev/antd'
import { Alert, Divider, Form, Input } from 'antd'

import { LineItemsEditor } from '@/components/line-items-editor'
import { PageForm } from '@/components/page'
import type { PurchaseOrder } from '@/types'

import { PurchaseFormFields } from './form-fields'
import { SalesOrderImport } from './sales-order-import'

/**
 * 编辑采购订单。只有草稿可改：已确认的采购单是给工厂的承诺，
 * 改价改量要留痕（T-24 的变更单）。
 */
export function PurchaseEdit() {
  const { formProps, saveButtonProps, form } = useForm<PurchaseOrder>({
    resource: 'purchase-orders',
    action: 'edit',
    redirect: 'show',
  })

  const status = Form.useWatch<number | undefined>('status', form)
  const locked = status !== undefined && status !== 10

  return (
    <PageForm
      action="edit"
      title="编辑采购订单"
      formProps={formProps}
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      notice={
        locked ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="这张采购单不是草稿，不能再改"
            description="已确认的采购单是对供应商的承诺。要调整请走变更单（T-24）；不要了可以改为已取消。"
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
        <PurchaseFormFields supplierReadOnly />

        <Divider orientation="left" style={{ margin: '8px 0 12px' }}>
          明细
        </Divider>

        {!locked && <SalesOrderImport form={form} />}

        <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
    </PageForm>
  )
}
