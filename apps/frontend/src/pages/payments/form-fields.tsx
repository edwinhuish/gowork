import { Form, Input, Select } from 'antd'

import type { PaymentInput } from '@/api/payments'
import { SETTLEMENT_METHOD_OPTIONS } from '@/constants/catalog'

/** 表单里的一行核销：选哪张单、核多少。 */
export interface PaymentAllocationFormValue {
  target_id?: number
  /** 定点字符串（后端 money.Decimal）。 */
  amount?: string
}

/**
 * 收付款表单的取值形状。
 *
 * 与 API 入参的差别只有一处：核销行不带 `target_type`——它由方向推出来
 * （收款核应收、付款核应付）。在表单里藏一个跟着方向变的隐式字段，
 * 改方向之后旧行会带着过期的类型，提交时才报错。
 */
export interface PaymentFormValues {
  direction: number
  partner_id?: number
  pay_date?: string
  currency_code?: string
  /** 定点字符串（后端 money.Decimal）。 */
  amount?: string
  method?: string
  reference?: string
  remark?: string
  allocations?: PaymentAllocationFormValue[]
}

/** 核销对象类型由方向决定，与后端 model.SettlementTarget 对齐。 */
export function targetTypeFor(direction: number): number {
  return direction === 20 ? 20 : 10
}

/** 把表单值整理成 API 入参：空字符串一律落成「留空」，由后端补默认值。 */
export function toPaymentInput(values: PaymentFormValues): PaymentInput {
  const targetType = targetTypeFor(values.direction)
  return {
    direction: values.direction,
    partner_id: values.partner_id,
    pay_date: values.pay_date?.trim() ?? '',
    currency_code: values.currency_code ?? '',
    // 留空（或 "0"）表示「取核销合计」；预收 / 预付款必须自己填，否则后端会拒。
    amount: values.amount ?? '',
    method: values.method ?? '',
    reference: values.reference?.trim() ?? '',
    remark: values.remark ?? '',
    allocations: (values.allocations ?? [])
      .filter((a) => a && a.target_id)
      .map((a) => ({
        target_type: targetType,
        target_id: a.target_id as number,
        amount: a.amount ?? '',
      })),
  }
}

/**
 * 日期、方式、水单号、备注。新建与编辑共用。
 *
 * 编辑页也只有这四个字段：金额与核销明细改了要回滚目标单据，
 * 「录错一笔」的正确操作是删掉重录。
 */
export function PaymentMetaFields() {
  return (
    <>
      <Form.Item
        name="pay_date"
        label="到账日"
        tooltip="钱实际到账 / 付出的那天，不是录入日。汇兑损益按这天的汇率算；留空取今天"
        rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
      >
        <Input placeholder="YYYY-MM-DD（留空取今天）" />
      </Form.Item>

      <Form.Item name="method" label="结算方式">
        <Select allowClear placeholder="选填" options={[...SETTLEMENT_METHOD_OPTIONS]} />
      </Form.Item>

      <Form.Item
        name="reference"
        label="水单号"
        tooltip="银行水单 / 流水号。外部号：只记录，不参与发号，也不要求唯一"
        rules={[{ max: 64, message: '最多 64 个字符' }]}
      >
        <Input placeholder="银行水单号，选填" maxLength={64} />
      </Form.Item>

      <Form.Item
        name="remark"
        label="备注"
        className="form-item-full"
        rules={[{ max: 255, message: '最多 255 个字符' }]}
      >
        <Input placeholder="款项来源、手续费说明等，选填" maxLength={255} />
      </Form.Item>
    </>
  )
}
