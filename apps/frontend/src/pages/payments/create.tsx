import { useList, useSelect } from '@refinedev/core'
import { Create, useForm } from '@refinedev/antd'
import { Button, Divider, Form, InputNumber, Radio, Select, Space, Typography } from 'antd'

import type { PaymentInput } from '@/api/payments'
import type { HttpError } from '@refinedev/core'
import { PAYMENT_DIRECTION_OPTIONS } from '@/constants/catalog'
import type { Currency, Partner, Payment, Settlement } from '@/types'

import { PaymentMetaFields, toPaymentInput, type PaymentFormValues } from './form-fields'

/**
 * 核销行编辑器。
 *
 * 目标单据只列未结清的（未结清 / 部分结清且还有余额），并且按方向取：
 * 收款只列应收、付款只列应付——方向错了后端也会拒，这里提前收窄可选项。
 * 选中单据后把金额预填成它剩下的余额，分批收付时再改小。
 */
function AllocationsEditor({ direction, partnerId }: { direction: number; partnerId?: number }) {
  const form = Form.useFormInstance()
  const isPayable = direction === 20

  const { result, query } = useList<Settlement>({
    resource: isPayable ? 'payables' : 'receivables',
    filters: partnerId ? [{ field: 'partner_id', operator: 'eq', value: partnerId }] : [],
    pagination: { mode: 'off' },
  })

  // 金额是定点字符串：未结余额只用于下拉提示与预填，真正的判定在服务端。
  const outstandingOf = (d: Settlement) => Number(d.amount) - Number(d.settled_amount)
  const docs = (result?.data ?? []).filter(
    (d) => (d.status === 10 || d.status === 20) && outstandingOf(d) > 0.0001,
  )
  const options = docs.map((d) => ({
    value: d.id,
    label: `${d.doc_no} · ${d.currency_code} ${outstandingOf(d).toFixed(2)} 未结`,
  }))

  return (
    <>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        {isPayable
          ? '付款只能核销应付单；不核销的部分是预付款。'
          : '收款只能核销应收单；不核销的部分是预收款。'}
        金额留空 = 全额核销。同一笔收付款的核销目标必须币种一致、往来单位一致。
      </Typography.Paragraph>

      <Form.List name="allocations">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name }) => (
              <Space key={key} align="baseline" wrap style={{ display: 'flex', marginBottom: 8 }}>
                <Form.Item
                  name={[name, 'target_id']}
                  rules={[{ required: true, message: '请选择要核销的单据' }]}
                  style={{ marginBottom: 0, minWidth: 380 }}
                >
                  <Select
                    showSearch
                    optionFilterProp="label"
                    loading={query.isLoading}
                    placeholder={isPayable ? '选择应付单' : '选择应收单'}
                    options={options}
                    onChange={(id: number) => {
                      const doc = docs.find((d) => d.id === id)
                      if (doc) {
                        // 预填走字符串：金额一旦经过 number，末位就开始漂。
                        form.setFieldValue(
                          ['allocations', name, 'amount'],
                          outstandingOf(doc).toFixed(4),
                        )
                      }
                    }}
                  />
                </Form.Item>

                <Form.Item
                  name={[name, 'amount']}
                  rules={[{ required: true, message: '填核销金额' }]}
                  style={{ marginBottom: 0 }}
                >
                  <InputNumber
                    style={{ width: 180 }}
                    min={0.0001}
                    step={0.01}
                    precision={4}
                    stringMode
                    placeholder="核销金额（原币）"
                  />
                </Form.Item>

                <Button type="link" danger onClick={() => remove(name)}>
                  移除
                </Button>
              </Space>
            ))}

            <Button type="dashed" block onClick={() => add({})}>
              + 核销一张单据
            </Button>
          </>
        )}
      </Form.List>
    </>
  )
}

/**
 * 登记一笔收款或付款。
 *
 * 表单里没有汇率：后端按**到账日**查汇率表。让人在这里填汇率，就等于允许
 * 把汇兑损益提前抹平——那笔差额记成 0 之后，报表里再也找不回来。
 */
export function PaymentCreate() {
  const { formProps, saveButtonProps, form, onFinish } = useForm<
    Payment,
    HttpError,
    PaymentInput
  >({
    resource: 'payments',
    action: 'create',
    redirect: 'show',
  })

  const direction = Form.useWatch<number>('direction', form) ?? 10
  const partnerId = Form.useWatch<number | undefined>('partner_id', form)
  const isPayable = direction === 20

  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: isPayable ? 'suppliers' : 'customers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { options: currencyOptions, query: currencyQuery } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'code',
    pagination: { mode: 'off' },
  })

  return (
    <Create saveButtonProps={saveButtonProps} title={isPayable ? '登记付款' : '登记收款'} goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{ direction: 10 }}
        // formProps.onFinish 的取值类型是 API 入参，而表单的核销行还少一个
        // target_type（由方向推出来），所以先补一次再交给 refine 的 mutation。
        onFinish={(values) => onFinish(toPaymentInput(values as unknown as PaymentFormValues))}
      >
        <Form.Item
          name="direction"
          label="方向"
          rules={[{ required: true, message: '选择收款还是付款' }]}
        >
          <Radio.Group
            optionType="button"
            options={[...PAYMENT_DIRECTION_OPTIONS]}
            onChange={() => {
              // 换方向等于换一套核销对象：留着上一方向选中的单据只会被后端拒，
              // 所以连同对方主体一起清掉。
              form.setFieldValue('partner_id', undefined)
              form.setFieldValue('allocations', [])
            }}
          />
        </Form.Item>

        <Form.Item
          name="partner_id"
          label={isPayable ? '供应商' : '客户'}
          tooltip="可以留空：核销目标会带出对方主体。预收 / 预付必须选"
        >
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            loading={partnerQuery.isLoading}
            placeholder={isPayable ? '选择供应商' : '选择客户'}
            options={partnerOptions}
          />
        </Form.Item>

        <Form.Item
          name="currency_code"
          label="币种"
          tooltip="留空随核销目标的币种；预收 / 预付留空则按本币。汇率一律取到账日的，不接受手填"
        >
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            loading={currencyQuery.isLoading}
            placeholder="留空随核销目标"
            options={currencyOptions}
          />
        </Form.Item>

        <Form.Item
          name="amount"
          label="金额（原币）"
          tooltip="留空 = 取核销合计。预收 / 预付必须自己填，否则会得到一张 0 元的单"
        >
          <InputNumber
            style={{ width: '100%' }}
            min={0}
            step={0.01}
            precision={4}
            stringMode
            placeholder="留空 = 取核销合计"
          />
        </Form.Item>

        <PaymentMetaFields />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          核销
        </Divider>

        {/* key 绑方向：换方向时整个编辑器重挂载，下拉里不会残留另一侧的候选单据。 */}
        <AllocationsEditor key={direction} direction={direction} partnerId={partnerId} />
      </Form>
    </Create>
  )
}
