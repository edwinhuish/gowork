import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  message,
} from 'antd'

import {
  INSPECTION_RESULT_OPTIONS,
  inspectionApi,
  type Inspection,
  type InspectionInput,
} from '@/api/inspections'
import { AttachmentPanel } from '@/components/attachment-panel'
import { Empty } from '@/components/fields'
import type { PurchaseOrderItem } from '@/types'
import { formatDate } from '@/utils/format'

/** 验货结论：合格 / 有条件接受 / 不合格，正好落在三档语义色上。 */
const RESULT_META: Record<string, { label: string; color?: string }> = {
  passed: { label: '合格', color: 'success' },
  failed: { label: '不合格', color: 'error' },
  partial: { label: '有条件接受', color: 'warning' },
}

interface FormValues {
  item?: number | 'all'
  result: Inspection['result']
  sample_rate?: number
  inspector?: string
  inspected_at?: string
  defects?: string
  remark?: string
}

/**
 * 采购详情里的验货面板（T-51b）。
 *
 * 这里补的是「我验过货」的证据：结论、抽检比例、缺陷项与附件照片。
 * 之前只有跟单节点上的一个完成日期，出了问题既证明不了验过，
 * 也说不清当时验出了什么。
 *
 * 不做编辑：验货是「某天某人验了这批货」的事实，改它等于改历史。
 */
export function InspectionPanel({
  orderId,
  items,
}: {
  orderId: number
  items: PurchaseOrderItem[]
}) {
  const [rows, setRows] = useState<Inspection[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [attach, setAttach] = useState<Inspection | null>(null)
  const [form] = Form.useForm<FormValues>()

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      setRows(await inspectionApi.list(orderId))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '验货记录加载失败')
    } finally {
      setLoading(false)
    }
  }, [orderId])

  useEffect(() => {
    void reload()
  }, [reload])

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ item: 'all', result: 'passed', sample_rate: 0 })
    setOpen(true)
  }

  const submit = async () => {
    const v = await form.validateFields()
    setSaving(true)
    try {
      const input: InspectionInput = {
        // 'all' 表示整单验货：不挂具体采购行。
        purchase_order_item_id: v.item && v.item !== 'all' ? Number(v.item) : null,
        result: v.result,
        sample_rate: v.sample_rate ?? 0,
        inspected_at: v.inspected_at,
        inspector: v.inspector,
        defects: v.defects,
        remark: v.remark,
      }
      await inspectionApi.create(orderId, input)
      message.success('验货记录已保存')
      setOpen(false)
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (record: Inspection) => {
    try {
      await inspectionApi.remove(record.id)
      message.success('验货记录已删除')
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      title="验货"
      size="small"
      style={{ marginTop: 16 }}
      extra={
        <Button size="small" onClick={openCreate}>
          记录一次验货
        </Button>
      }
    >
      <Table<Inspection>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: '还没有验货记录。「我验过货」这句话，需要这里的一条记录来支撑。' }}
        columns={[
          {
            title: '验货日',
            dataIndex: 'inspected_at',
            width: 110,
            render: (v: string) => formatDate(v),
          },
          {
            title: '货物',
            dataIndex: 'sku_code',
            width: 140,
            render: (v: string) => (v ? <span className="cell-mono">{v}</span> : <span className="cell-sub">整单</span>),
          },
          {
            title: '结论',
            dataIndex: 'result',
            width: 120,
            render: (v: string) => {
              const meta = RESULT_META[v]
              return meta ? <Tag color={meta.color}>{meta.label}</Tag> : <Tag>{v}</Tag>
            },
          },
          {
            title: '抽检',
            dataIndex: 'sample_rate',
            width: 80,
            align: 'right' as const,
            render: (v: number) => (v > 0 ? `${v}%` : <span className="cell-sub">—</span>),
          },
          {
            title: '验货人',
            dataIndex: 'inspector',
            width: 100,
            render: (v: string) => v || <span className="cell-sub">—</span>,
          },
          {
            title: '缺陷项',
            dataIndex: 'defects',
            render: (v: string) =>
              v ? <span title={v}>{v.length > 60 ? `${v.slice(0, 60)}…` : v}</span> : <Empty />,
          },
          {
            title: '操作',
            width: 130,
            render: (_: unknown, record) => (
              <Space size={4}>
                <Button size="small" type="link" onClick={() => setAttach(record)}>
                  附件
                </Button>
                <Popconfirm
                  title="删除这条验货记录？"
                  description="附件会一起失去挂靠点。录错了删掉重录，审计里两次都在。"
                  okText="删除"
                  cancelText="返回"
                  onConfirm={() => void remove(record)}
                >
                  <Button size="small" type="link" danger>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title="记录一次验货"
        width={640}
        onCancel={() => setOpen(false)}
        onOk={() => void submit()}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="item"
            label="验哪一批货"
            tooltip="整单验货选「整单」；分批从不同工厂出来时按行记，客户问起才说得清"
          >
            <Select
              options={[
                { value: 'all', label: '整单' },
                ...items.map((it) => ({ value: it.id, label: `${it.sku_code} · ${it.name_cn}` })),
              ]}
            />
          </Form.Item>

          <Space size={12} style={{ display: 'flex' }}>
            <Form.Item name="result" label="结论" rules={[{ required: true, message: '请选择结论' }]}>
              <Select options={INSPECTION_RESULT_OPTIONS} style={{ width: 220 }} />
            </Form.Item>
            <Form.Item name="sample_rate" label="抽检比例 %" tooltip="0 表示未记录">
              <InputNumber min={0} max={100} style={{ width: 120 }} />
            </Form.Item>
            <Form.Item name="inspector" label="验货人">
              <Input style={{ width: 140 }} />
            </Form.Item>
          </Space>

          <Form.Item name="inspected_at" label="验货日期">
            <DatePicker
              style={{ width: 200 }}
              onChange={(_, dateString) => form.setFieldValue('inspected_at', String(dateString))}
              placeholder="留空取今天"
            />
          </Form.Item>

          <Form.Item
            name="defects"
            label="缺陷项"
            rules={[{ max: 1000, message: '最多 1000 个字符' }]}
            tooltip="写清哪一处、多少个、客户是否接受；这一段是要给客户看的"
          >
            <Input.TextArea rows={3} placeholder="如：第 12 箱有 3 件焊点开裂，客户同意让价 2% 后接受" />
          </Form.Item>

          <Form.Item name="remark" label="备注" rules={[{ max: 255, message: '最多 255 个字符' }]}>
            <Input />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={!!attach}
        title={attach ? `验货附件 · ${formatDate(attach.inspected_at)}` : '验货附件'}
        width={820}
        footer={null}
        onCancel={() => setAttach(null)}
        forceRender
      >
        {attach ? (
          /* 附件挂在验货记录上而不是采购单：一张采购单可能验三次，
             混在采购单上就分不清哪张照片对应哪次验货。 */
          <AttachmentPanel entityType="inspection" entityId={attach.id} title="照片与报告" />
        ) : null}
      </Modal>
    </Card>
  )
}
