import { Edit, useForm } from '@refinedev/antd'
import { Alert, Divider, Form, Input } from 'antd'

import type { Contract } from '@/types'

import { ContractTermsEditor } from './terms-editor'

/**
 * 编辑合同。
 *
 * 只有草稿可改。可改的只有签订日、交期、备注与分期；
 * 来源、客户与金额不在表单里：合同金额来自来源单据，
 * 要改金额就改那张单据、作废这张合同重新生成。
 */
export function ContractEdit() {
  const { formProps, saveButtonProps, form } = useForm<Contract>({
    resource: 'contracts',
    action: 'edit',
    redirect: 'show',
  })

  // 这几个字段不在表单里渲染，但 initialValues 带着整条记录，useWatch 能读到。
  const status = Form.useWatch<number | undefined>('status', form)
  const total = Form.useWatch<string | undefined>('amount_total', form)
  const currencyCode = Form.useWatch<string | undefined>('currency_code', form)
  const locked = status !== undefined && status !== 10

  return (
    <Edit
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      title="编辑合同"
      goBack={null}
    >
      {locked && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="这张合同不是草稿，不能再改"
          description="已确认的合同是对客户的承诺。要改请先取消这张，再从来源单据重新生成一份。"
        />
      )}

      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="来源、客户与金额不可修改"
        description="金额与汇率是生成那一刻从来源单据抄下来并锁定的。要改金额请改来源单据后重新生成。"
      />

      <Form {...formProps} layout="vertical" className="entity-form">
        {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。
            必须是注册过的字段——antd 只提交已注册的项，光靠 initialValues
            带着 updated_at 是不够的，它会在提交时被丢掉。 */}
        <Form.Item name="updated_at" hidden>
          <Input />
        </Form.Item>
        <Form.Item
          name="sign_date"
          label="签订日"
          rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
          extra="留空保持原签订日——它已经印在单号与发出去的 PDF 上"
        >
          <Input placeholder="2026-10-28" disabled={locked} />
        </Form.Item>

        <Form.Item
          name="delivery_date"
          label="约定交期"
          rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: '格式应为 YYYY-MM-DD' }]}
          extra="留空表示清空交期"
        >
          <Input placeholder="可留空" disabled={locked} />
        </Form.Item>

        <Form.Item name="remark" label="备注" className="form-item-full">
          <Input.TextArea
            rows={2}
            maxLength={255}
            showCount
            disabled={locked}
            placeholder="包装、唛头、特殊条款等"
          />
        </Form.Item>

        <Divider orientation="left" className="form-item-full" style={{ margin: '8px 0 16px' }}>
          付款分期
        </Divider>

        <div className="form-item-full">
          <ContractTermsEditor total={total} currencyCode={currencyCode} />
        </div>
      </Form>
    </Edit>
  )
}
