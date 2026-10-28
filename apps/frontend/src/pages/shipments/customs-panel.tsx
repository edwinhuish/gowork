import { useState } from 'react'
import { useInvalidate } from '@refinedev/core'
import { useTable } from '@refinedev/antd'
import {
  AutoComplete,
  Button,
  Card,
  Col,
  Form,
  Input,
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

import { customsDeclarationApi, type CustomsDeclarationInput } from '@/api/customs-declarations'
import { Empty } from '@/components/fields'
import {
  CUSTOMS_STATUS_OPTIONS,
  SUPERVISION_MODE_OPTIONS,
  customsStatusColor,
  customsStatusLabel,
} from '@/constants/catalog'
import type { CustomsDeclaration } from '@/types'
import { formatDate } from '@/utils/format'

interface DeclForm {
  status: number
  decl_no?: string
  decl_date?: string
  supervision_mode?: string
  remark?: string
}

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/

/**
 * 出货详情页的报关单面板。
 *
 * 一票可以有多张：不同 HS 编码分票申报、被退单后改单重报都是新的一条。
 * 旧的必须留着——"海关退回过什么"只有旧单还在的时候才回答得了，
 * 所以这张表不限制条数，也不在主表上另开几个字段做覆盖。
 */
export function CustomsPanel({ shipmentId }: { shipmentId: number }) {
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<CustomsDeclaration | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm<DeclForm>()
  const invalidate = useInvalidate()

  const { tableProps } = useTable<CustomsDeclaration>({
    resource: 'customs-declarations',
    filters: {
      permanent: [{ field: 'shipment_id', operator: 'eq', value: shipmentId }],
    },
  })

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ status: 10, supervision_mode: '0110' })
    setModalOpen(true)
  }

  const openEdit = (record: CustomsDeclaration) => {
    setEditing(record)
    form.setFieldsValue({
      status: record.status,
      decl_no: record.decl_no,
      decl_date: record.decl_date ? record.decl_date.slice(0, 10) : '',
      supervision_mode: record.supervision_mode,
      remark: record.remark,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const body: CustomsDeclarationInput = {
      shipment_id: shipmentId,
      status: values.status,
      decl_no: values.decl_no?.trim() ?? '',
      decl_date: values.decl_date?.trim() || undefined,
      supervision_mode: values.supervision_mode?.trim() ?? '',
      remark: values.remark?.trim() ?? '',
    }

    setSubmitting(true)
    try {
      if (editing) {
        await customsDeclarationApi.update(editing.id, body)
        message.success('报关单已更新')
      } else {
        await customsDeclarationApi.create(body)
        message.success('已新增报关单')
      }
      setModalOpen(false)
      form.resetFields()
      await invalidate({ resource: 'customs-declarations', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const remove = async (record: CustomsDeclaration) => {
    try {
      await customsDeclarationApi.remove(record.id)
      message.success('已删除报关单')
      await invalidate({ resource: 'customs-declarations', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      title="报关单"
      style={{ marginTop: 16 }}
      extra={
        <Button type="primary" onClick={openCreate}>
          新增报关单
        </Button>
      }
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        一票货可以有多张：不同 HS 编码分票申报，被退单后改单重报也是新的一条。
        报关单号是海关给的外部号，只记录、不参与发号。
      </Typography.Paragraph>

      <Table
        {...tableProps}
        rowKey="id"
        size="small"
        scroll={{ x: 900 }}
        locale={{ emptyText: '还没有报关记录。要素备齐后在这里登记，报关单号可以后补。' }}
      >
        <Table.Column
          dataIndex="status"
          title="状态"
          width={100}
          render={(value: number) => (
            <Tag color={customsStatusColor(value)}>{customsStatusLabel(value)}</Tag>
          )}
        />
        <Table.Column
          dataIndex="decl_no"
          title="报关单号"
          width={180}
          render={(value: string) => (value ? <span className="cell-mono">{value}</span> : <Empty />)}
        />
        <Table.Column
          dataIndex="decl_date"
          title="申报日期"
          width={120}
          render={(value: string | null) => (value ? formatDate(value) : <Empty />)}
        />
        <Table.Column
          dataIndex="supervision_mode"
          title="监管方式"
          width={110}
          render={(value: string) => (value ? <Tag>{value}</Tag> : <Empty />)}
        />
        <Table.Column dataIndex="remark" title="备注" render={(value: string) => value || <Empty />} />
        <Table.Column
          title="操作"
          width={140}
          render={(_, record: CustomsDeclaration) => (
            <Space size={4}>
              <Button size="small" type="link" onClick={() => openEdit(record)}>
                编辑
              </Button>
              <Popconfirm
                title="删除这张报关单？"
                description="退单重报的场景请保留旧单，否则「海关退回过什么」查不到"
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
      </Table>

      <Modal
        open={modalOpen}
        title={editing ? '编辑报关单' : '新增报关单'}
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
                name="status"
                label="状态"
                rules={[{ required: true, message: '请选择状态' }]}
              >
                <Select options={[...CUSTOMS_STATUS_OPTIONS]} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="supervision_mode"
                label="监管方式"
                tooltip="海关四位代码；候选项只是常用的几种，可以直接输入其它代码"
              >
                <AutoComplete
                  options={[...SUPERVISION_MODE_OPTIONS]}
                  placeholder="0110"
                  filterOption={(input, option) =>
                    (option?.label ?? '').toLowerCase().includes(input.toLowerCase()) ||
                    (option?.value ?? '').includes(input)
                  }
                />
              </Form.Item>
            </Col>

            <Col span={12}>
              <Form.Item
                name="decl_no"
                label="报关单号"
                tooltip="海关给的 18 位号；待申报时可以先留空"
                rules={[{ max: 32, message: '最多 32 个字符' }]}
              >
                <Input placeholder="后补也可以" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="decl_date"
                label="申报日期"
                rules={[
                  {
                    validator: (_, value?: string) => {
                      const v = value?.trim()
                      if (!v || DATE_PATTERN.test(v)) return Promise.resolve()
                      return Promise.reject(new Error('格式应为 YYYY-MM-DD'))
                    },
                  },
                ]}
              >
                <Input placeholder="YYYY-MM-DD" />
              </Form.Item>
            </Col>

            <Col span={24}>
              <Form.Item name="remark" label="备注">
                <Input placeholder="退单原因、改单说明，选填" maxLength={255} />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </Card>
  )
}
