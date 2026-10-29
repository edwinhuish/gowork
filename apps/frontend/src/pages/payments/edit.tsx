import type { HttpError } from '@refinedev/core'
import { Edit, useForm } from '@refinedev/antd'
import { Alert, Form, Input } from 'antd'

import type { PaymentUpdateInput } from '@/api/payments'
import type { Payment } from '@/types'

import { PaymentMetaFields } from './form-fields'

/**
 * 编辑收付款单。
 *
 * 只能改日期、方式、水单号、备注：金额或核销明细改了，要先回滚目标单据的
 * 已核销金额再按新值重算状态——那件事的正确入口是删掉重录，不是就地改。
 */
export function PaymentEdit() {
  const { formProps, saveButtonProps, query } = useForm<Payment, HttpError, PaymentUpdateInput>({
    resource: 'payments',
    action: 'edit',
    redirect: 'show',
  })

  return (
    <Edit
      saveButtonProps={saveButtonProps}
      isLoading={query?.isLoading}
      title={`编辑收付款${formProps.initialValues?.doc_no ? ` · ${formProps.initialValues.doc_no}` : ''}`}
    >
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="金额与核销明细不能在这里改"
        description="录错了请删除整张单重录：删单会把目标单据的已核销金额与状态一起回滚。"
      />

      <Form {...formProps} layout="vertical" className="entity-form">

        {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}

        <Form.Item name="updated_at" hidden>
          <Input />

        </Form.Item>
        <PaymentMetaFields />
      </Form>
    </Edit>
  )
}
