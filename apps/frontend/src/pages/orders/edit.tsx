import { Edit, useForm } from '@refinedev/antd'
import { Alert, Divider, Form } from 'antd'

import { LineItemsEditor } from '@/components/line-items-editor'
import type { SalesOrder } from '@/types'

import { CreditHint } from './credit-hint'
import { OrderFormFields } from './form-fields'

/**
 * 编辑销售订单。只有草稿可改：已确认的订单是对客户的承诺，
 * 改动要留痕（T-24 的变更单），不是把原单悄悄改掉。
 */
export function OrderEdit() {
  const { formProps, saveButtonProps, form } = useForm<SalesOrder>({
    resource: 'sales-orders',
    action: 'edit',
    redirect: 'show',
  })

  // status 不在表单里渲染，但 initialValues 带着整条记录，useWatch 能读到。
  const status = Form.useWatch<number | undefined>('status', form)
  const locked = status !== undefined && status !== 10

  return (
    <Edit
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      title="编辑销售订单"
      goBack={null}
    >
      {locked && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="这张订单不是草稿，不能再改"
          description="已确认的订单是对客户的承诺。要调整请走变更单（T-24）；不要了可以改为已取消。"
        />
      )}

      <Form {...formProps} layout="vertical" className="entity-form">
        <OrderFormFields partnerReadOnly />

        {/* 草稿改完还要确认，额度这一刻就该看见；改明细不会占额度，所以只是提示。 */}
        <CreditHint partnerId={Form.useWatch<number | undefined>('partner_id', form)} />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          明细
        </Divider>

        <LineItemsEditor currencyCode={Form.useWatch<string | undefined>('currency_code', form)} />
      </Form>
    </Edit>
  )
}
