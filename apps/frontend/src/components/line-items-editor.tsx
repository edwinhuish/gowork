import { useMemo } from 'react'
import { useList } from '@refinedev/core'
import { Button, Col, Form, Input, InputNumber, Row, Select, Tag, Typography } from 'antd'

import type { Product, ProductSource } from '@/types'

export interface LineItemRow {
  product_id?: number
  qty?: number
  unit_price?: number
  source_id?: number
  /** 采购单专有：这行是为哪张销售单的哪一行备的（以销定采）。 */
  sales_order_item_id?: number
  remark?: string
}

/**
 * 单据明细编辑器：报价单与销售订单共用。
 *
 * 只让填「数量 × 单价」（报价单还可指定成本货源），金额一律不在这里提交——
 * 行小计只用来给人看，真正落库的金额由后端按单据汇率算。
 *
 * 为什么抽出来共用：报价单与订单的明细行字段几乎一样，抄一份的结果是
 * 哪天改了数量精度或产品搜索方式，只改了其中一个页面。
 */
export function LineItemsEditor({
  currencyCode,
  showSource = false,
}: {
  currencyCode?: string
  /** 报价单需要选成本货源；订单没有成本口径，不显示这一列。 */
  showSource?: boolean
}) {
  const form = Form.useFormInstance()
  const rows = Form.useWatch<LineItemRow[]>('items', form) ?? []

  const { query: productsQuery } = useList<Product>({
    resource: 'products',
    pagination: { mode: 'off' },
  })
  // 订单用不到货源，但 hook 不能条件调用，用 enabled 把它停掉。
  const { query: sourcesQuery } = useList<ProductSource>({
    resource: 'product-sources',
    pagination: { mode: 'off' },
    queryOptions: { enabled: showSource },
  })

  const products = productsQuery.data?.data ?? []
  const sources = sourcesQuery.data?.data ?? []

  const productOptions = useMemo(
    () =>
      products.map((p) => ({
        value: p.id,
        label: `${p.sku_code} · ${p.name_cn}`,
      })),
    [products],
  )

  const sourcesByProduct = useMemo(() => {
    const map = new Map<number, ProductSource[]>()
    for (const s of sources) {
      if (s.status !== 1) continue
      const list = map.get(s.product_id) ?? []
      list.push(s)
      map.set(s.product_id, list)
    }
    return map
  }, [sources])

  const onProductChange = (index: number, productId: number) => {
    const product = products.find((p) => p.id === productId)
    const current = (form.getFieldValue(['items', index, 'unit_price']) as number | undefined) ?? 0
    // 售价作为报价的起点：报价单九成是"按目录价再谈"，从 0 开始填等于让人重抄一遍。
    if (!current && product) {
      // 产品售价是字符串（定点），报价明细的单价还是 number（T-37 后续片再改）。
      form.setFieldValue(['items', index, 'unit_price'], Number(product.sale_price))
    }
    // 换了产品，上一行选的货源就不属于这个产品了，必须清掉。
    form.setFieldValue(['items', index, 'source_id'], undefined)
  }

  const spans = showSource
    ? { product: 7, qty: 3, price: 3, source: 6, subtotal: 3, action: 2 }
    : { product: 10, qty: 4, price: 4, source: 0, subtotal: 4, action: 2 }

  return (
    <div>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
        {showSource
          ? '成本取所选货源的本币采购价；不选则用该产品本币价最低的启用货源，保存时锁进明细。'
          : '金额由后端按「数量 × 单价 × 单据汇率」算，前端提交的合计不参与。'}
      </Typography.Paragraph>

      <Row gutter={8} style={{ marginBottom: 4 }}>
        <Col span={spans.product}>
          <Typography.Text type="secondary">产品</Typography.Text>
        </Col>
        <Col span={spans.qty}>
          <Typography.Text type="secondary">数量</Typography.Text>
        </Col>
        <Col span={spans.price}>
          <Typography.Text type="secondary">单价</Typography.Text>
        </Col>
        {showSource && (
          <Col span={spans.source}>
            <Typography.Text type="secondary">成本货源</Typography.Text>
          </Col>
        )}
        <Col span={spans.subtotal}>
          <Typography.Text type="secondary">行小计</Typography.Text>
        </Col>
        <Col span={spans.action} />
      </Row>

      <Form.List name="items">
        {(fields, { add, remove }) => (
          <>
            {fields.map((field) => {
              const row = rows[field.name] ?? {}
              const rowSources = row.product_id
                ? (sourcesByProduct.get(row.product_id) ?? [])
                : []
              const subtotal = (row.qty ?? 0) * (row.unit_price ?? 0)

              return (
                <Row gutter={8} key={field.key} align="top">
                  <Col span={spans.product}>
                    <Form.Item
                      name={[field.name, 'product_id']}
                      rules={[{ required: true, message: '请选择产品' }]}
                    >
                      <Select
                        placeholder="选择产品"
                        showSearch
                        options={productOptions}
                        loading={productsQuery.isLoading}
                        onChange={(value: number) => onProductChange(field.name, value)}
                        filterOption={(input, option) =>
                          String(option?.label ?? '')
                            .toLowerCase()
                            .includes(input.toLowerCase())
                        }
                      />
                    </Form.Item>
                    {/* 不渲染就会在提交时丢掉「从销售订单导入」带进来的关联 */}
                    <Form.Item name={[field.name, 'sales_order_item_id']} hidden>
                      <Input />
                    </Form.Item>
                    {row.sales_order_item_id ? (
                      <Tag color="blue" style={{ marginTop: -18 }}>
                        以销定采
                      </Tag>
                    ) : null}
                  </Col>
                  <Col span={spans.qty}>
                    <Form.Item
                      name={[field.name, 'qty']}
                      rules={[{ required: true, message: '数量' }]}
                    >
                      <InputNumber style={{ width: '100%' }} min={0.001} step={1} placeholder="0" />
                    </Form.Item>
                  </Col>
                  <Col span={spans.price}>
                    <Form.Item name={[field.name, 'unit_price']}>
                      <InputNumber
                        style={{ width: '100%' }}
                        min={0}
                        step={0.01}
                        precision={4}
                        placeholder="0.00"
                      />
                    </Form.Item>
                  </Col>
                  {showSource && (
                    <Col span={spans.source}>
                      <Form.Item name={[field.name, 'source_id']}>
                        <Select
                          placeholder={
                            row.product_id && rowSources.length === 0 ? '该产品还没有货源' : '最低价'
                          }
                          allowClear
                          disabled={!row.product_id || rowSources.length === 0}
                          options={rowSources.map((s) => ({
                            value: s.id,
                            label: `${s.partner?.name ?? '供应商'} · ${Number(s.purchase_price_base).toFixed(
                              2,
                            )}${s.supplier_sku ? ` · ${s.supplier_sku}` : ''}`,
                          }))}
                        />
                      </Form.Item>
                    </Col>
                  )}
                  <Col span={spans.subtotal}>
                    <div style={{ paddingTop: 6, fontVariantNumeric: 'tabular-nums' }}>
                      {subtotal.toLocaleString('zh-CN', {
                        minimumFractionDigits: 2,
                        maximumFractionDigits: 2,
                      })}
                      {currencyCode ? ` ${currencyCode}` : ''}
                    </div>
                  </Col>
                  <Col span={spans.action}>
                    <Button type="link" danger onClick={() => remove(field.name)}>
                      删除
                    </Button>
                  </Col>
                </Row>
              )
            })}

            <Button
              type="dashed"
              block
              onClick={() => add({ qty: 1, unit_price: 0 })}
              style={{ marginTop: 4 }}
            >
              加一行明细
            </Button>
          </>
        )}
      </Form.List>
    </div>
  )
}
