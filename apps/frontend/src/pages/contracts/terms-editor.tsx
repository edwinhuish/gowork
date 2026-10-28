import { Button, Col, Form, Input, InputNumber, Row, Typography } from 'antd'

export interface ContractTermRow {
  name?: string
  /** 金额按字符串收发（后端 money.Decimal）。 */
  amount?: string
  due_date?: string
  trigger?: string
}

/**
 * 付款分期编辑器：新建与编辑共用。
 *
 * 只提交原币金额与付款条件，本币金额由后端按合同汇率折算——两个币种都让前端填，
 * 等于允许它们互相对不上。下面的合计与差额只用于提示，
 * 真正的判据在服务端：合计不等于合同总额直接拒绝保存。
 */
export function ContractTermsEditor({
  total,
  currencyCode,
}: {
  /** 合同总额（原币，定点字符串）；未知时不显示差额判定，只显示合计。 */
  total?: string
  currencyCode?: string
}) {
  const form = Form.useFormInstance()
  const rows = Form.useWatch<ContractTermRow[]>('terms', form) ?? []

  // 下面的合计与差额只用于提示，且只在显示这一层转 number：
  // 真正的判据在服务端（合计必须等于总额，定点比到末位）。
  const sum = rows.reduce((acc, row) => acc + Number(row?.amount ?? 0), 0)
  const diff = Number(total ?? 0) - sum
  const matched = total !== undefined && Math.abs(diff) < 0.005
  const money = (v: number | string) =>
    Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

  return (
    <div>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
        分期合计必须等于合同总额
        {total !== undefined ? ` ${money(total)}${currencyCode ? ` ${currencyCode}` : ''}` : ''}
        。本币金额由后端按合同汇率折算，不在前端提交。
      </Typography.Paragraph>

      <Row gutter={8} style={{ marginBottom: 4 }}>
        <Col span={6}>
          <Typography.Text type="secondary">期次名称</Typography.Text>
        </Col>
        <Col span={4}>
          <Typography.Text type="secondary">金额</Typography.Text>
        </Col>
        <Col span={4}>
          <Typography.Text type="secondary">付款日</Typography.Text>
        </Col>
        <Col span={8}>
          <Typography.Text type="secondary">付款条件</Typography.Text>
        </Col>
        <Col span={2} />
      </Row>

      <Form.List name="terms">
        {(fields, { add, remove }) => (
          <>
            {fields.map((field) => (
              <Row gutter={8} key={field.key} align="top">
                <Col span={6}>
                  <Form.Item
                    name={[field.name, 'name']}
                    rules={[{ required: true, message: '请填写期次名称' }]}
                  >
                    <Input placeholder="订金 30%" maxLength={64} />
                  </Form.Item>
                </Col>
                <Col span={4}>
                  <Form.Item
                    name={[field.name, 'amount']}
                    rules={[{ required: true, message: '请填写金额' }]}
                  >
                    <InputNumber
                      style={{ width: '100%' }}
                      min={0}
                      step={0.01}
                      precision={4}
                      stringMode
                      placeholder="0.00"
                    />
                  </Form.Item>
                </Col>
                <Col span={4}>
                  <Form.Item
                    name={[field.name, 'due_date']}
                    rules={[{ pattern: /^\d{4}-\d{2}-\d{2}$/, message: 'YYYY-MM-DD' }]}
                  >
                    <Input placeholder="可留空" />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name={[field.name, 'trigger']}>
                    <Input placeholder="见提单副本后 7 天内" maxLength={128} />
                  </Form.Item>
                </Col>
                <Col span={2}>
                  <Button
                    type="link"
                    danger
                    disabled={fields.length <= 1}
                    onClick={() => remove(field.name)}
                  >
                    删除
                  </Button>
                </Col>
              </Row>
            ))}

            <Row gutter={8} align="middle" style={{ marginTop: 4 }}>
              <Col span={12}>
                <Button type="dashed" block onClick={() => add({ amount: '0' })}>
                  加一期
                </Button>
              </Col>
              <Col span={12} style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                <Typography.Text type={matched ? 'success' : 'danger'}>
                  分期合计 {money(sum)}
                  {currencyCode ? ` ${currencyCode}` : ''}
                  {total !== undefined && !matched
                    ? diff > 0
                      ? `，还差 ${money(diff)}`
                      : `，超出 ${money(-diff)}`
                    : ''}
                </Typography.Text>
              </Col>
            </Row>
          </>
        )}
      </Form.List>
    </div>
  )
}
