import { useEffect, useState } from 'react'
import { useOne, useSelect } from '@refinedev/core'
import { Create, useForm } from '@refinedev/antd'
import { Alert, Divider, Form, Input, Select, Tag } from 'antd'

import {
  CONTRACT_SOURCE_TYPE_OPTIONS,
  CONTRACT_TYPE_OPTIONS,
  orderStatusLabel,
  quotationStatusLabel,
} from '@/constants/catalog'
import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { Contract, Quotation, SalesOrder } from '@/types'

import { ContractTermsEditor } from './terms-editor'

/** 今天，YYYY-MM-DD。签订日默认今天：合同多半是当场出具。 */
function today(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * 生成合同。
 *
 * 没有"空白新建"：客户、币种、汇率与总额全部照抄来源单据，
 * 所以第一步永远是选来源。分期在下面按整单金额预填一期，需要时再拆。
 *
 * 只列已确认的报价单与订单，是为了少给两个点不动的选项；
 * 真正的拦截在后端（草稿、已取消、过期报价一律 409）。
 */
export function ContractCreate() {
  // 合同是凭以收款的文件，重复出具会让客户收到两份不同单号的 PI。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps, form } = useForm<Contract>({
    resource: 'contracts',
    action: 'create',
    redirect: 'show',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
  })

  const sourceType = Form.useWatch<number | undefined>('source_type', form) ?? 10
  const sourceId = Form.useWatch<number | undefined>('source_id', form)
  const isOrder = sourceType === 20
  const sourceResource = isOrder ? 'sales-orders' : 'quotations'

  const { options: sourceOptions, query: sourceListQuery } = useSelect<Quotation | SalesOrder>({
    resource: sourceResource,
    optionLabel: (item) =>
      `${item.doc_no} · ${item.partner?.name ?? '未知客户'} · ${item.amount_total} ` +
      `${item.currency_code} · ${isOrder ? orderStatusLabel(item.status) : quotationStatusLabel(item.status)}`,
    optionValue: 'id',
    pagination: { mode: 'off' },
    // 报价单只列已确认的：草稿还没发给客户，价格随时可改。
    filters: isOrder ? [] : [{ field: 'status', operator: 'eq', value: 20 }],
  })

  const { query: sourceQuery } = useOne<Quotation | SalesOrder>({
    resource: sourceResource,
    id: sourceId ?? 0,
    queryOptions: { enabled: Boolean(sourceId) },
  })
  const source = sourceQuery.data?.data

  // 选中来源后按整单金额预填一期"全额"：绝大多数合同一次性付清，
  // 需要分期再拆。切换来源必须重填，否则会把上一张单的金额留在表单里。
  const [prefilled, setPrefilled] = useState<number>()
  useEffect(() => {
    if (!source || prefilled === source.id) return
    setPrefilled(source.id)
    form.setFieldValue('terms', [{ name: '全额', amount: source.amount_total }])
  }, [source, prefilled, form])

  return (
    <Create saveButtonProps={saveButtonProps} title="生成合同" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{
          type: 10,
          source_type: 10,
          sign_date: today(),
          terms: [{ name: '全额', amount: 0 }],
        }}
      >
        <Form.Item
          name="type"
          label="合同类型"
          rules={[{ required: true, message: '请选择合同类型' }]}
          tooltip="PI 常先于订单成立（客户凭它付订金或开信用证）；销售合同是双方签字的正式合同"
        >
          <Select options={CONTRACT_TYPE_OPTIONS} />
        </Form.Item>

        <Form.Item
          name="source_type"
          label="来源单据类型"
          rules={[{ required: true, message: '请选择来源类型' }]}
        >
          <Select
            options={CONTRACT_SOURCE_TYPE_OPTIONS}
            onChange={() => {
              // 换了来源类型，上一张单的来源与金额都不再适用。
              form.setFieldValue('source_id', undefined)
              form.setFieldValue('terms', [{ name: '全额', amount: 0 }])
              setPrefilled(undefined)
            }}
          />
        </Form.Item>

        <Form.Item
          name="source_id"
          label={isOrder ? '来源销售订单' : '来源报价单'}
          className="form-item-full"
          rules={[{ required: true, message: '请选择来源单据' }]}
          extra="合同不存货物明细：客户、币种、汇率与总额照抄这张单据，保存后不可改"
        >
          <Select
            placeholder={isOrder ? '选择已确认的销售订单' : '选择已确认的报价单'}
            showSearch
            options={sourceOptions}
            loading={sourceListQuery.isLoading}
            filterOption={(input, option) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
          />
        </Form.Item>

        {source && (
          <div className="form-item-full">
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 20 }}
              message={
                <span>
                  来源 <Tag>{source.doc_no}</Tag> 客户{' '}
                  {source.partner?.name ?? '—'}，总额{' '}
                  <strong>
                    {/* 来源总额是定点的字符串，这里只为显示转一次 number。 */}
                    {Number(source.amount_total).toLocaleString('zh-CN', {
                      minimumFractionDigits: 2,
                      maximumFractionDigits: 2,
                    })}{' '}
                    {source.currency_code}
                  </strong>
                  ，汇率 {source.exchange_rate}
                </span>
              }
              description="换算本币用的是来源单据锁定那一刻的汇率，不取今天的汇率。"
            />
          </div>
        )}

        <Form.Item
          name="sign_date"
          label="签订日"
          rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
          tooltip="单号年月按它取；留空取今天"
        >
          <Input placeholder="2026-10-28" />
        </Form.Item>

        <Form.Item
          name="delivery_date"
          label="约定交期"
          rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
          extra="留空沿用来源单据的交期"
        >
          <Input placeholder="可留空" />
        </Form.Item>

        <Form.Item name="remark" label="备注" className="form-item-full">
          <Input.TextArea rows={2} maxLength={255} showCount placeholder="包装、唛头、特殊条款等" />
        </Form.Item>

        <Divider orientation="left" className="form-item-full" style={{ margin: '8px 0 16px' }}>
          付款分期
        </Divider>

        <div className="form-item-full">
          <ContractTermsEditor
            total={source?.amount_total}
            currencyCode={source?.currency_code}
          />
        </div>
      </Form>
    </Create>
  )
}
