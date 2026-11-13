import { useForm } from '@refinedev/antd'
import { Button, Divider, Form, Space } from 'antd'

import { PageForm } from '@/components/page'
import { LineItemsEditor } from '@/components/line-items-editor'
import { useIdempotencyKey } from '@/hooks/use-idempotency-key'
import type { Quotation } from '@/types'

import { QuotationFormFields, defaultValidUntil } from './form-fields'
import { QuotationPreviewAlert, useQuotationPreview } from './preview'

export function QuotationCreate() {
  // 同一次提交的重试复用同一个键：请求超时后再点一次，服务端回放上次结果，
  // 不会真的再建一版报价。成功后换新键，下一次新建才算另一次操作。
  const { key: idempotencyKey, rotate: rotateIdempotencyKey } = useIdempotencyKey()
  const { formProps, saveButtonProps, form } = useForm<Quotation>({
    resource: 'quotations',
    action: 'create',
    redirect: 'show',
    mutationMeta: { idempotencyKey },
    onMutationSuccess: () => rotateIdempotencyKey(),
  })
  const { preview, loading, run } = useQuotationPreview(form)

  return (
    <PageForm
      action="create"
      title="新建报价单"
      formProps={{
        ...formProps,
        initialValues: {
        trade_term: 'FOB',
        valid_until: defaultValidUntil(),
        items: [{ qty: '1', unit_price: '0' }],
        },
      }}
      saveButtonProps={saveButtonProps}
      wide
    >
      <QuotationFormFields />

      <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
        明细
      </Divider>

      <LineItemsEditor
        showSource
        currencyCode={Form.useWatch<string | undefined>('currency_code', form)}
      />

      <Space style={{ marginTop: 12 }}>
        <Button onClick={run} loading={loading} htmlType="button">
          试算毛利
        </Button>
      </Space>
      {/* 试算结果不提交，只是让人在点保存前知道自己报的价赚不赚 */}
      <QuotationPreviewAlert preview={preview} />
    </PageForm>
  )
}
