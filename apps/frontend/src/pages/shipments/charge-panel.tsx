import { useCallback, useEffect, useState } from 'react'
import { useInvalidate, useSelect } from '@refinedev/core'
import { useTable } from '@refinedev/antd'
import {
  Button,
  Card,
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

import { shipmentChargeApi, type ShipmentChargeInput } from '@/api/shipment-charges'
import { Empty, Money } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import { toCardPager } from '@/components/partner-cards'
import { SHIPMENT_CHARGE_OPTIONS, shipmentChargeLabel } from '@/constants/catalog'
import type { Currency, Partner, ShipmentCharge } from '@/types'

interface ChargeForm {
  type: number
  partner_id?: number
  /** 定点字符串（后端 money.Decimal）：折算在服务端做。 */
  amount: string
  currency_code?: string
  remark?: string
}

/**
 * 出货详情页的运费杂费面板。
 *
 * 费用不单独占一个侧栏菜单：它永远归属"这一票货的成本"，离开票据语境单看一张
 * 费用清单，既不知道付给谁，也算不出单票毛利。
 *
 * 不限制出货状态：货代账单晚于出货到达是常态，要求草稿才能录费用等于让人
 * 把已经发出的货退回草稿。这与"出货数量只能在草稿改"是两条规则。
 */
export function ChargesPanel({ shipmentId }: { shipmentId: number }) {
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ShipmentCharge | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [totalBase, setTotalBase] = useState<string | null>(null)
  const [form] = Form.useForm<ChargeForm>()
  const invalidate = useInvalidate()

  const { tableProps } = useTable<ShipmentCharge>({
    resource: 'shipment-charges',
    filters: {
      permanent: [{ field: 'shipment_id', operator: 'eq', value: shipmentId }],
    },
  })

  // 合计由后端算：列表分页，手里这一页不是全部。写操作后要显式重取。
  const loadSummary = useCallback(async () => {
    try {
      const res = await shipmentChargeApi.summary(shipmentId)
      setTotalBase(res.total_base)
    } catch {
      setTotalBase(null)
    }
  }, [shipmentId])

  useEffect(() => {
    void loadSummary()
  }, [loadSummary])

  // 收款方只能是货代或供应商，分两组给：混在一个平铺下拉里，
  // 运营分不清"中远"是货代还是工厂。后端也会校验，这里只是少撞一次 400。
  const { options: forwarderOptions } = useSelect<Partner>({
    resource: 'forwarders',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })
  const { options: supplierOptions } = useSelect<Partner>({
    resource: 'suppliers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })
  const payeeOptions = [
    ...(forwarderOptions.length ? [{ label: '货代', options: forwarderOptions }] : []),
    ...(supplierOptions.length ? [{ label: '供应商', options: supplierOptions }] : []),
  ]

  const { options: currencyOptions, query: currencyQuery } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'code',
    pagination: { mode: 'off' },
  })

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ type: 10, amount: '0' })
    setModalOpen(true)
  }

  const openEdit = (record: ShipmentCharge) => {
    setEditing(record)
    form.setFieldsValue({
      type: record.type,
      partner_id: record.partner_id ?? undefined,
      amount: record.amount,
      currency_code: record.currency_code,
      remark: record.remark,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const body: ShipmentChargeInput = {
      shipment_id: shipmentId,
      type: values.type,
      partner_id: values.partner_id ?? null,
      amount: values.amount ?? '',
      currency_code: values.currency_code || undefined,
      remark: values.remark?.trim() ?? '',
    }

    setSubmitting(true)
    try {
      if (editing) {
        await shipmentChargeApi.update(editing.id, body)
        message.success('费用已更新')
      } else {
        await shipmentChargeApi.create(body)
        message.success('已新增费用')
      }
      setModalOpen(false)
      form.resetFields()
      await invalidate({ resource: 'shipment-charges', invalidates: ['list'] })
      await loadSummary()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const remove = async (record: ShipmentCharge) => {
    try {
      await shipmentChargeApi.remove(record.id)
      message.success('已删除费用')
      await invalidate({ resource: 'shipment-charges', invalidates: ['list'] })
      await loadSummary()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      title="运费杂费"
      style={{ marginTop: 16 }}
      extra={
        <Space size={12}>
          <span className="cell-sub">
            折本币合计：{totalBase === null ? '—' : <Money value={totalBase} />}
          </span>
          <Button type="primary" onClick={openCreate}>
            新增费用
          </Button>
        </Space>
      }
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        只登记我方承担的费用。FOB 下客户付的海运费不要录 —— 录进来会被算成我方成本，
        毛利凭空低一截而钱一分没花。外币金额按录入当日汇率折本币并锁定。
      </Typography.Paragraph>

      <ResponsiveTable
        tableProps={tableProps}
        rows={(tableProps.dataSource ?? []) as ShipmentCharge[]}
        loading={tableProps.loading === true}
        pager={toCardPager(tableProps.pagination, (total) => `共 ${total} 条`)}
        rowKey="id"
        size="small"
        scrollX={960}
        empty="还没有费用记录。货代账单到手后在这里补录。"
      >
        <Table.Column
          dataIndex="type"
          title="类型"
          width={110}
          render={(value: number) => <Tag>{shipmentChargeLabel(value) ?? <Empty />}</Tag>}
        />
        <Table.Column
          dataIndex="partner"
          title="收款方"
          width={200}
          render={(value: ShipmentCharge['partner']) =>
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
          dataIndex="amount"
          title="金额（原币）"
          width={170}
          align="right"
          render={(value: string, record: ShipmentCharge) => (
            <Space size={6}>
              <Money value={value} />
              <Tag>{record.currency_code}</Tag>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="amount_base"
          title="折本币"
          width={130}
          align="right"
          render={(value: string) => <Money value={value} />}
        />
        <Table.Column
          dataIndex="exchange_rate"
          title="汇率"
          width={100}
          align="right"
          render={(value: string) =>
            Number(value) === 1 ? <Empty /> : Number(value).toFixed(4)
          }
        />
        <Table.Column dataIndex="remark" title="备注" render={(value: string) => value || <Empty />} />
        <Table.Column
          title="操作"
          width={140}
          render={(_, record: ShipmentCharge) => (
            <Space size={4}>
              <Button size="small" type="link" onClick={() => openEdit(record)}>
                编辑
              </Button>
              <Popconfirm
                title="删除这条费用？"
                description="删除后单票成本与毛利会立即变化"
                okText="删除"
                cancelText="取消"
                onConfirm={() => remove(record)}
              >
                <Button size="small" type="link" danger>
                  删除
                </Button>
              </Popconfirm>
            </Space>
          )}
        />
      </ResponsiveTable>

      <Modal
        open={modalOpen}
        title={editing ? '编辑费用' : '新增费用'}
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
                name="type"
                label="费用类型"
                rules={[{ required: true, message: '请选择类型' }]}
              >
                <Select options={[...SHIPMENT_CHARGE_OPTIONS]} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="partner_id"
                label="收款方"
                tooltip="货代或供应商；账单只来一个总数时可以先留空"
              >
                <Select placeholder="未指定" allowClear showSearch options={payeeOptions} optionFilterProp="label" />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                name="amount"
                label="金额（原币）"
              >
                <InputNumber
                  style={{ width: '100%' }}
                  min={0}
                  step={0.01}
                  precision={2}
                  stringMode
                  placeholder="0.00"
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
                  optionFilterProp="label"
                />
              </Form.Item>
            </Col>

            <Col span={24}>
              <Form.Item name="remark" label="备注">
                <Input placeholder="账单号、费用说明，选填" maxLength={255} />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </Card>
  )
}
