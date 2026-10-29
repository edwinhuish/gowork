import { Button, Col, Form, Input, InputNumber, Row, Typography } from 'antd'

import { ProductSelect, type ProductHint } from '@/components/product-select'
import { STOCK_DOC_COUNT } from '@/constants/catalog'
import { formatQty } from '@/utils/format'

export interface StockDocItemRow {
  product_id?: number
  /** 编辑回显用：明细行上有当时的 SKU 与品名快照，不必再问一次服务端。 */
  sku_code?: string
  name_cn?: string
  /** 数量按字符串收发（后端 money.Decimal）：3 位小数，前端不算账。 */
  qty?: string
  remark?: string
}

/**
 * 出入库单明细编辑器。
 *
 * 数量这一列的语义随单据类型变：入库 / 出库是数量，盘点是**实盘数**。
 * 盘点的账面数不在这里填——它必须取确认那一刻的库存，
 * 否则盘点单搁半天，中途别的单据动过货，算出来的盈亏就是错的。
 */
export function StockDocItemsEditor({ docType }: { docType?: number }) {
  const form = Form.useFormInstance()
  const rows = Form.useWatch<StockDocItemRow[]>('items', form) ?? []
  const isCount = docType === STOCK_DOC_COUNT

  const initialProductOf = (row: StockDocItemRow): ProductHint | null =>
    row.product_id && row.sku_code
      ? { id: row.product_id, sku_code: row.sku_code, name_cn: row.name_cn }
      : null

  const spans = { product: 10, qty: 4, remark: 6, action: 2 }

  return (
    <div>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
        {isCount
          ? '盘点是先填实盘数：确认时按「实盘 − 当时账面」生成盈亏流水，账实相符的行不产生流水。'
          : '确认后才计入库存；草稿阶段怎么改都不影响库存。'}
      </Typography.Paragraph>

      <Row gutter={8} style={{ marginBottom: 4 }}>
        <Col span={spans.product}>
          <Typography.Text type="secondary">产品</Typography.Text>
        </Col>
        <Col span={spans.qty}>
          <Typography.Text type="secondary">{isCount ? '实盘数' : '数量'}</Typography.Text>
        </Col>
        <Col span={spans.remark}>
          <Typography.Text type="secondary">备注</Typography.Text>
        </Col>
        <Col span={spans.action} />
      </Row>

      <Form.List
        name="items"
        rules={[
          {
            validator: async (_, value: unknown[]) => {
              if (!value || value.length === 0) {
                throw new Error('至少加一行明细')
              }
            },
          },
        ]}
      >
        {(fields, { add, remove }, { errors }) => (
          <>
            {fields.map((field) => (
              <Row gutter={8} key={field.key} align="top">
                <Col span={spans.product}>
                  <Form.Item
                    name={[field.name, 'product_id']}
                    rules={[{ required: true, message: '请选择产品' }]}
                  >
                    <ProductSelect
                      placeholder="搜 SKU / 品名 / 客户编号"
                      initialProduct={initialProductOf(rows[field.name] ?? {})}
                    />
                  </Form.Item>
                </Col>
                <Col span={spans.qty}>
                  <Form.Item
                    name={[field.name, 'qty']}
                    rules={[
                      { required: true, message: '填数量' },
                      {
                        // 盘点可以填 0（盘到没有货是结论），入库 / 出库填 0 说明这行没写完。
                        // 数量是定点字符串，判断前先转 number；真正的拦截在服务端。
                        validator: async (_, value: string | undefined) => {
                          if (value === undefined || value === null || value === '') return
                          const num = Number(value)
                          if (!isCount && num <= 0) {
                            throw new Error('数量必须大于 0')
                          }
                          if (num < 0) {
                            throw new Error('数量不能是负数')
                          }
                        },
                      },
                    ]}
                  >
                    <InputNumber
                      style={{ width: '100%' }}
                      min={isCount ? 0 : 0.001}
                      step={1}
                      precision={3}
                      stringMode
                      placeholder="0"
                    />
                  </Form.Item>
                </Col>
                <Col span={spans.remark}>
                  <Form.Item name={[field.name, 'remark']}>
                    <Input maxLength={255} placeholder="批号、货位等" />
                  </Form.Item>
                </Col>
                <Col span={spans.action}>
                  <Button type="link" danger onClick={() => remove(field.name)}>
                    删除
                  </Button>
                </Col>
              </Row>
            ))}

            <Button type="dashed" block onClick={() => add({ qty: isCount ? '0' : '1' })}>
              加一行明细
            </Button>

            <Form.ErrorList errors={errors} />

            {rows.length > 0 && !isCount ? (
              <Typography.Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
                合计 {formatQty(rows.reduce((sum, r) => sum + Number(r.qty ?? 0), 0))}
              </Typography.Text>
            ) : null}
          </>
        )}
      </Form.List>
    </div>
  )
}
