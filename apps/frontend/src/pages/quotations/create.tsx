import { Create, useForm } from '@refinedev/antd'
import { Button, Divider, Form, Space } from 'antd'

import type { Quotation } from '@/types'

import { QuotationFormFields, defaultValidUntil } from './form-fields'
import { QuotationItemsEditor } from './items-editor'
import { QuotationPreviewAlert, useQuotationPreview } from './preview'

export function QuotationCreate() {
  const { formProps, saveButtonProps, form } = useForm<Quotation>({
    resource: 'quotations',
    action: 'create',
    redirect: 'show',
  })
  const { preview, loading, run } = useQuotationPreview(form)

  return (
    <Create saveButtonProps={saveButtonProps} title="新建报价单" goBack={null}>
      <Form
        {...formProps}
        layout="vertical"
        className="entity-form"
        initialValues={{
          trade_term: 'FOB',
          valid_until: defaultValidUntil(),
          items: [{ qty: 1, unit_price: 0 }],
        }}
      >
        <QuotationFormFields />

        <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
          明细
        </Divider>

        <QuotationItemsEditor
          currencyCode={Form.useWatch<string | undefined>('currency_code', form)}
        />

        <Space style={{ marginTop: 12 }}>
          <Button onClick={run} loading={loading} htmlType="button">
            试算毛利
          </Button>
        </Space>
        {/* 试算结果不提交，只是让人在点保存前知道自己报的价赚不赚 */}
        <QuotationPreviewAlert preview={preview} />
      </Form>
    </Create>
  )
}
