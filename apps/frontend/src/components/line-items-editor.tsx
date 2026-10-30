import { Button, Col, Form, Grid, Input, InputNumber, Row, Typography } from 'antd'

import { AccentTag } from '@/components/fields'
import { ProductSelect, type ProductHint } from '@/components/product-select'
import { SourceSelect } from '@/components/product-source-select'

export interface LineItemRow {
  product_id?: number
  /**
   * 编辑回显用：后端把明细上的产品关联一起回吐。
   * 产品后来改名了也不影响——行上的 sku_code / name_cn 是当时的快照。
   */
  product?: ProductHint | null
  sku_code?: string
  name_cn?: string
  /** 数量按字符串收发（后端 money.Decimal）：与单价一样，过 number 就丢末位。 */
  qty?: string
  /** 单价按字符串收发（后端 money.Decimal），过一遍 JS number 就丢了末位。 */
  unit_price?: string
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

  /**
   * 行的回显信息：优先用后端带回的产品关联，其次用行上的 SKU 快照。
   * 两者都没有（新建的空行）时返回 null，由下拉自己按 id 兜底查一次。
   */
  const initialProductOf = (row: LineItemRow): ProductHint | null => {
    if (row.product) return row.product
    if (row.product_id && row.sku_code) {
      return { id: row.product_id, sku_code: row.sku_code, name_cn: row.name_cn }
    }
    return null
  }

  const onProductChange = (index: number, product?: ProductHint) => {
    const current = form.getFieldValue(['items', index, 'unit_price']) as string | undefined
    // 售价作为报价的起点：报价单九成是"按目录价再谈"，从 0 开始填等于让人重抄一遍。
    // 产品售价与报价单价现在都是定点的字符串，直接搬过去，不做 number 中转。
    if (!current && product?.sale_price) {
      form.setFieldValue(['items', index, 'unit_price'], product.sale_price)
    }
    // 换了产品，上一行选的货源就不属于这个产品了，必须清掉。
    form.setFieldValue(['items', index, 'source_id'], undefined)
  }

  // 窄屏那一行 5~6 列挤在 375px 里：产品只剩 130px、数量单价各 50px，
  // 「删除」连自己的 60px 都挤不出来（实机量到出界 9px）。改成竖着排：
  // 产品整行，数量 / 单价各半行，小计与删除同行（金额靠左、按钮靠右）。
  const narrow = Grid.useBreakpoint().md === false
  const spans = narrow
    ? { product: 24, qty: 12, price: 12, source: 24, subtotal: 12, action: 12 }
    : showSource
      ? { product: 7, qty: 3, price: 3, source: 6, subtotal: 3, action: 2 }
      : { product: 10, qty: 4, price: 4, source: 0, subtotal: 4, action: 2 }

  return (
    <div>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
        {showSource
          ? '成本取所选货源的本币采购价；不选则用该产品本币价最低的启用货源，保存时锁进明细。'
          : '金额由后端按「数量 × 单价 × 单据汇率」算，前端提交的合计不参与。'}
      </Typography.Paragraph>

      {/* 表头标签只在宽屏给：窄屏竖排之后这行标签会各占一行、与下面错位。
          窄屏靠每个输入的 placeholder 自报家门。 */}
      {!narrow && (
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
      )}

      <Form.List name="items">
        {(fields, { add, remove }) => (
          <>
            {fields.map((field) => {
              const row = rows[field.name] ?? {}
              // 只用于界面提示：真实金额由服务端按「数量 × 单价 × 汇率」算，
              // 这里转成 number 只是为了让用户边填边看个大概。
              const subtotal = Number(row.qty ?? 0) * Number(row.unit_price ?? 0)

              return (
                <Row gutter={8} key={field.key} align="top">
                  <Col span={spans.product}>
                    <Form.Item
                      name={[field.name, 'product_id']}
                      rules={[{ required: true, message: '请选择产品' }]}
                    >
                      <ProductSelect
                        placeholder="搜 SKU / 品名 / 客户编号"
                        initialProduct={initialProductOf(row)}
                        onPick={(product) => onProductChange(field.name, product)}
                      />
                    </Form.Item>
                    {/* 不渲染就会在提交时丢掉「从销售订单导入」带进来的关联 */}
                    <Form.Item name={[field.name, 'sales_order_item_id']} hidden>
                      <Input />
                    </Form.Item>
                    {row.sales_order_item_id ? (
                      <AccentTag style={{ marginTop: -18 }}>
                        以销定采
                      </AccentTag>
                    ) : null}
                  </Col>
                  <Col span={spans.qty}>
                    <Form.Item
                      name={[field.name, 'qty']}
                      rules={[{ required: true, message: '数量' }]}
                    >
                      <InputNumber
                        style={{ width: '100%' }}
                        min={0.001}
                        step={1}
                        precision={3}
                        stringMode
                        placeholder={narrow ? '数量' : '0'}
                      />
                    </Form.Item>
                  </Col>
                  <Col span={spans.price}>
                    <Form.Item name={[field.name, 'unit_price']}>
                      <InputNumber
                        style={{ width: '100%' }}
                        min={0}
                        step={0.01}
                        precision={4}
                        stringMode
                        placeholder={narrow ? '单价' : '0.00'}
                      />
                    </Form.Item>
                  </Col>
                  {showSource && (
                    <Col span={spans.source}>
                      <Form.Item name={[field.name, 'source_id']}>
                        <SourceSelect productId={row.product_id} />
                      </Form.Item>
                    </Col>
                  )}
                  <Col span={spans.subtotal}>
                    <div style={{ paddingTop: 6, fontVariantNumeric: 'tabular-nums' }}>
                      {/* 窄屏没有表头那行，光秃秃一个金额不知道是什么。 */}
                      {narrow ? <span className="cell-sub">小计 </span> : null}
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
              onClick={() => add({ qty: '1', unit_price: '0' })}
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
