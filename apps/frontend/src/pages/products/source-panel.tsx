import { useState } from 'react'
import { useInvalidate, useSelect } from '@refinedev/core'
import { useTable } from '@refinedev/antd'
import {
  Button,
  Card,
  Checkbox,
  Col,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'

import { productSourceApi, type ProductSourceInput } from '@/api/product-sources'
import { Empty, Mono, Money } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import { toCardPager } from '@/components/partner-cards'
import type { Currency, Partner, ProductSource } from '@/types'

interface SourceForm {
  partner_id: number
  supplier_sku?: string
  purchase_price: string
  currency_code?: string
  moq: number
  delivery_days: number
  remark?: string
}

/**
 * 产品详情页的货源面板。
 *
 * 货源不单独占一个侧栏菜单：它的第一层归属永远是"这个产品的成本从哪来"，
 * 离开产品语境单看一张货源清单，既不知道是哪家的什么货，也没法比价。
 *
 * 列表按本币价升序（后端排序）：这张表唯一要回答的问题是"从谁家进最便宜"。
 */
export function SourcesPanel({ productId }: { productId: number }) {
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ProductSource | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm<SourceForm>()
  // 写操作走 http 而不是 refine 的 mutation 钩子，因此要显式让列表失效重查。
  const invalidate = useInvalidate()

  const [showDeleted, setShowDeleted] = useState(false)
  const { tableProps, setFilters } = useTable<ProductSource>({
    resource: 'product-sources',
    filters: {
      permanent: [{ field: 'product_id', operator: 'eq', value: productId }],
    },
  })

  // 永久筛选之外只有这一个条件，变化时整份写回而不是逐个合并：
  // 关掉开关时才能确保 include_deleted 真的不再出现在查询里。
  //
  // 在**事件里**发，不要放进 useEffect：refine 的 setFilters 依赖
  // filters.permanent 这个数组，而这里传的是内联字面量 —— 每次渲染都是新数组，
  // setFilters 跟着每次都是新引用。放进 effect 依赖里就会自激：effect →
  // setFilters → 重渲染 → effect …… 产品详情页实测每进一次报 8 次
  // "Maximum update depth exceeded"。开关本来就是事件，事件里发最省事。
  const toggleDeleted = (checked: boolean) => {
    setShowDeleted(checked)
    setFilters(
      checked ? [{ field: 'include_deleted', operator: 'eq', value: true }] : [],
      'replace',
    )
  }

  // 供应商下拉取 /suppliers 而不是 /partners：货源指向的主体必须是供应商角色，
  // 后端也会校验，这里只是不让用户先选一个客户再撞一次 400。
  const { options: supplierOptions, query: supplierQuery } = useSelect<Partner>({
    resource: 'suppliers',
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

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ purchase_price: '0', moq: 0, delivery_days: 0 })
    setModalOpen(true)
  }

  const openEdit = (record: ProductSource) => {
    setEditing(record)
    form.setFieldsValue({
      partner_id: record.partner_id,
      supplier_sku: record.supplier_sku,
      purchase_price: record.purchase_price,
      currency_code: record.currency_code,
      moq: record.moq,
      delivery_days: record.delivery_days,
      remark: record.remark,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const body: ProductSourceInput = {
      product_id: productId,
      partner_id: values.partner_id,
      supplier_sku: values.supplier_sku?.trim() ?? '',
      purchase_price: values.purchase_price ?? '0',
      currency_code: values.currency_code || undefined,
      moq: values.moq ?? 0,
      delivery_days: values.delivery_days ?? 0,
      remark: values.remark?.trim() ?? '',
    }

    setSubmitting(true)
    try {
      if (editing) {
        await productSourceApi.update(editing.id, body)
        message.success('货源已更新')
      } else {
        await productSourceApi.create(body)
        message.success('已新增货源')
      }
      setModalOpen(false)
      form.resetFields()
      await invalidate({ resource: 'product-sources', invalidates: ['list'] })
    } catch (err) {
      // 重复货号是 409，后端文案已经说清是哪个货号，直接透出即可。
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const remove = async (record: ProductSource) => {
    try {
      await productSourceApi.remove(record.id)
      message.success('已删除货源')
      await invalidate({ resource: 'product-sources', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const restore = async (record: ProductSource) => {
    try {
      await productSourceApi.restore(record.id)
      message.success('已恢复货源')
      await invalidate({ resource: 'product-sources', invalidates: ['list'] })
    } catch (err) {
      // 货号被别的货源占了是 409，后端文案写明了是哪个货号。
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  return (
    <Card
      title="产品货源"
      style={{ marginTop: 16 }}
      extra={
        <Space size={8}>
          {/* 删除是软删：这一行的恢复入口藏在开关后面，
              平时列表里看不到它们。 */}
          <Checkbox
            checked={showDeleted}
            onChange={(e) => toggleDeleted(e.target.checked)}
          >
            含已删除
          </Checkbox>
          <Button type="primary" onClick={openCreate}>
            新增货源
          </Button>
        </Space>
      }
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        采购价按录入当日汇率折算本币，列表按本币价升序；同一供应商下货号不可重复。
      </Typography.Paragraph>

      <ResponsiveTable
        tableProps={tableProps}
        rows={(tableProps.dataSource ?? []) as ProductSource[]}
        loading={tableProps.loading === true}
        pager={toCardPager(tableProps, (total) => `共 ${total} 条`)}
        rowKey="id"
        size="small"
        scrollX={980}
        empty="还没有货源记录，先挂一家供应商的报价"
      >
        <Table.Column
          dataIndex="partner"
          title="供应商"
          width={200}
          render={(value: ProductSource['partner']) =>
            value ? (
              <div className="cell-stack">
                <span className="cell-strong">{value.name}</span>
                <span className="cell-sub">{value.code}</span>
              </div>
            ) : (
              <Empty />
            )
          }
        />
        <Table.Column
          dataIndex="supplier_sku"
          title="供应商货号"
          width={150}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="purchase_price"
          title="采购价"
          width={170}
          align="right"
          render={(value: number, record: ProductSource) => (
            <Space size={6}>
              <Money value={value} />
              <Tag>{record.currency_code}</Tag>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="purchase_price_base"
          title="折本币"
          width={130}
          align="right"
          render={(value: number) => <Money value={value} />}
        />
        <Table.Column
          dataIndex="moq"
          title="起订量"
          width={100}
          align="right"
          render={(value: number) => (value ? value.toLocaleString('zh-CN') : <Empty />)}
        />
        <Table.Column
          dataIndex="delivery_days"
          title="交期"
          width={100}
          align="right"
          render={(value: number) => (value ? `${value} 天` : <Empty />)}
        />
        <Table.Column
          title="操作"
          width={140}
          render={(_, record: ProductSource) => (
            <Space size={4}>
              {record.deleted_at ? (
                // 已删除的货源只能做恢复：编辑与删除对它都是 404。
                <Button size="small" type="link" onClick={() => restore(record)}>
                  恢复
                </Button>
              ) : (
                <>
                  <Button size="small" type="link" onClick={() => openEdit(record)}>
                    编辑
                  </Button>
                  <Popconfirm
                    title="删除这条货源？"
                    description="已锁定成本快照的历史报价不受影响"
                    okText="删除"
                    cancelText="取消"
                    onConfirm={() => remove(record)}
                  >
                    <Button size="small" type="link" danger>
                      删除
                    </Button>
                  </Popconfirm>
                </>
              )}
            </Space>
          )}
        />
      </ResponsiveTable>

      <Modal
        open={modalOpen}
        title={editing ? '编辑货源' : '新增货源'}
        okText="保存"
        cancelText="取消"
        width={640}
        confirmLoading={submitting}
        onOk={submit}
        onCancel={() => {
          setModalOpen(false)
          form.resetFields()
        }}
      >
        <Form form={form} layout="vertical">
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item
                name="partner_id"
                label="供应商"
                rules={[{ required: true, message: '请选择供应商' }]}
                extra={editing ? '货源不能换供应商，需要换请新建一条' : undefined}
              >
                <Select
                  placeholder="请选择"
                  showSearch
                  disabled={editing !== null}
                  options={supplierOptions}
                  loading={supplierQuery.isLoading}
                  filterOption={(input, option) =>
                    String(option?.label ?? '')
                      .toLowerCase()
                      .includes(input.toLowerCase())
                  }
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="supplier_sku"
                label="供应商货号"
                rules={[{ max: 64, message: '最多 64 个字符' }]}
                extra="对方目录里的叫法；还没给号可以先留空"
              >
                <Input placeholder="A-100" />
              </Form.Item>
            </Col>

            <Col span={12}>
              {/* stringMode：本币价由后端用定点算，前端不过一遍 number。 */}
              <Form.Item name="purchase_price" label="采购价（原币）" initialValue="0">
                <InputNumber
                  style={{ width: '100%' }}
                  min={0}
                  step={0.01}
                  precision={4}
                  stringMode
                  placeholder="0.0000"
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="currency_code" label="币种" tooltip="留空按本币，非本币按当日汇率折本币">
                <Select
                  placeholder="本币"
                  allowClear
                  showSearch
                  options={currencyOptions}
                  loading={currencyQuery.isLoading}
                  filterOption={(input, option) =>
                    String(option?.label ?? '')
                      .toLowerCase()
                      .includes(input.toLowerCase())
                  }
                />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item name="moq" label="最小起订量">
                <InputNumber style={{ width: '100%' }} min={0} step={100} placeholder="0 表示不限" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="delivery_days" label="交期（天）">
                <InputNumber style={{ width: '100%' }} min={0} max={3650} placeholder="0 表示未确认" />
              </Form.Item>
            </Col>

            <Col span={24}>
              <Form.Item name="remark" label="备注">
                <Input placeholder="选填" maxLength={255} />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </Card>
  )
}
