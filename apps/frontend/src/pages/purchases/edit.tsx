import { Edit, useForm } from '@refinedev/antd'
import { Alert, Divider, Form } from 'antd'

import { LineItemsEditor } from '@/components/line-items-editor'
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
    <Edit
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      title="编辑采购订单"
      goBack={null}
    >
      {locked && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="这张采购单不是草稿，不能再改"
          description="已确认的采购单是对供应商的承诺。要调整请走变更单（T-24）；不要了可以改为已取消。"
        />
      )}

      <Form {...formProps} layout="vertical" className="entity-form">
        <PurchaseFormFields supplierReadOnly />

        <Divider orientation="left" style={{ margin: '8px 0 12px' }}>
          明细
        </Divider>

        {!locked && <SalesOrderImport form={form} />}

        <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
      </Form>
    </Edit>
  )
}
