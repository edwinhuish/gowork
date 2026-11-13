import { useForm } from '@refinedev/antd'
import { Alert, Button, Divider, Form, Input, Space } from 'antd'

import { PageForm } from '@/components/page'
import { LineItemsEditor } from '@/components/line-items-editor'
import type { Quotation } from '@/types'

import { QuotationFormFields } from './form-fields'
import { QuotationPreviewAlert, useQuotationPreview } from './preview'

/**
 * 编辑报价单。
 *
 * 只有草稿可改：已确认的那一版是发给客户的承诺，改它等于改历史。
 * 后端会拒绝（409），这里提前把按钮禁掉并说明该怎么做，省一次无意义的失败。
 */
export function QuotationEdit() {
  const { formProps, saveButtonProps, form } = useForm<Quotation>({
    resource: 'quotations',
    action: 'edit',
    redirect: 'show',
  })
  const { preview, loading, run } = useQuotationPreview(form)

  // status 不在表单里渲染，但 initialValues 带着整条记录，
  // useWatch 能读到它——不必再向 refine 要一次记录。
  const status = Form.useWatch<number | undefined>('status', form)
  const locked = status !== undefined && status !== 10

  return (
    <PageForm
      action="edit"
      title="编辑报价单"
      formProps={formProps}
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      notice={
        locked ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="这张报价单不是草稿，不能再改"
            description="已确认的报价单是发给客户的承诺。要改请回详情页「另存新版本」，新版从草稿开始改。"
          />
        ) : null
      }
      wide
    >
      {/* 乐观锁：把打开页面时读到的版本原样带回服务端。
          必须是注册过的字段——antd 只提交已注册的项，光靠 initialValues 里带着
          updated_at 是不够的，它会在提交时被丢掉。 */}
      <Form.Item name="updated_at" hidden>
        <Input />
      </Form.Item>
      <QuotationFormFields partnerReadOnly />

      <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
        明细
      </Divider>

      <LineItemsEditor
        showSource
        currencyCode={Form.useWatch<string | undefined>('currency_code', form)}
      />

      <Space style={{ marginTop: 12 }}>
        <Button onClick={run} loading={loading} htmlType="button" disabled={locked}>
          试算毛利
        </Button>
      </Space>
      <QuotationPreviewAlert preview={preview} />
    </PageForm>
  )
}
