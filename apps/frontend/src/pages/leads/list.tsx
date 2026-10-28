import { useState } from 'react'
import type { CrudFilters } from '@refinedev/core'
import { useInvalidate } from '@refinedev/core'
import { CreateButton, DeleteButton, EditButton, List, useTable } from '@refinedev/antd'
import { Button, Form, Input, message, Modal, Select, Space, Table, Tag } from 'antd'

import { leadApi } from '@/api/leads'
import { Empty, Mono } from '@/components/fields'
import {
  LEAD_SOURCE_OPTIONS,
  LEAD_STATUS_OPTIONS,
  leadSourceLabel,
  leadStatusLabel,
} from '@/constants/catalog'
import type { Lead } from '@/types'

interface LeadSearch {
  keyword?: string
  status?: number
  source?: string
  open_only?: number
}

interface ConvertForm {
  code: string
  role?: number
}

/**
 * 线索列表。
 *
 * 转化用弹窗只问一件事：主体编码。编码会印在报价单与发票上，
 * 自动生成的 CUS0007 三个月后没人看得出是哪家；其余字段从线索带过去，不让人重填。
 */
export function LeadList() {
  const [converting, setConverting] = useState<Lead | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm<ConvertForm>()
  const invalidate = useInvalidate()

  const { tableProps, searchFormProps } = useTable<Lead>({
    resource: 'leads',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as LeadSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.status) {
        filters.push({ field: 'status', operator: 'eq', value: v.status })
      }
      if (v.source) {
        filters.push({ field: 'source', operator: 'eq', value: v.source })
      }
      if (v.open_only) {
        filters.push({ field: 'open_only', operator: 'eq', value: 1 })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  const submitConvert = async () => {
    if (!converting) return
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      const res = await leadApi.convert(converting.id, values)
      message.success(`已转为往来单位 ${res.partner.code}`)
      // 同名同国只提示不阻断，提示必须显式弹出来，否则用户不会去看那条新建的记录。
      if (res.warning) message.warning(res.warning, 6)
      setConverting(null)
      form.resetFields()
      invalidate({ resource: 'leads', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '转化失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <List headerButtons={<CreateButton>新建线索</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索公司 / 联系人 / 电话" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="全部状态" allowClear style={{ width: 130 }} options={LEAD_STATUS_OPTIONS} />
        </Form.Item>
        <Form.Item name="source">
          <Select placeholder="全部来源" allowClear style={{ width: 150 }} options={LEAD_SOURCE_OPTIONS} />
        </Form.Item>
        <Form.Item name="open_only">
          <Select
            placeholder="全部线索"
            allowClear
            style={{ width: 140 }}
            options={[{ value: 1, label: '只看还在跟' }]}
          />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">
              查询
            </Button>
            <Button onClick={resetFilters}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <Table {...tableProps} rowKey="id" scroll={{ x: 1150 }}>
        <Table.Column dataIndex="company" title="公司 / 询盘主体" width={200} />
        <Table.Column dataIndex="country" title="国家" width={110} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="contact_name" title="联系人" width={110} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="phone" title="电话" width={150} render={(v: string) => <Mono>{v}</Mono>} />
        <Table.Column
          dataIndex="source"
          title="来源"
          width={120}
          render={(v: string) => <Tag>{leadSourceLabel(v)}</Tag>}
        />
        <Table.Column
          dataIndex="status"
          title="状态"
          width={100}
          render={(v: number) => {
            const color = v === 30 ? 'success' : v === 40 ? 'default' : 'processing'
            return <Tag color={color}>{leadStatusLabel(v)}</Tag>
          }}
        />
        <Table.Column
          dataIndex="partner"
          title="已转为主体"
          width={150}
          render={(value: Lead['partner']) =>
            value ? <Mono>{value.code}</Mono> : <Empty />
          }
        />
        <Table.Column
          title="操作"
          width={220}
          render={(_, record: Lead) => (
            <Space size={4}>
              <EditButton size="small" recordItemId={record.id} resource="leads" />
              {record.status !== 30 && (
                <Button
                  size="small"
                  type="link"
                  onClick={() => {
                    setConverting(record)
                    form.setFieldsValue({ code: '', role: 1 })
                  }}
                >
                  转化
                </Button>
              )}
              <DeleteButton size="small" recordItemId={record.id} resource="leads" />
            </Space>
          )}
        />
      </Table>

      <Modal
        open={converting !== null}
        title={converting ? `把「${converting.company}」转为往来单位` : '转化线索'}
        okText="确认转化"
        cancelText="取消"
        confirmLoading={submitting}
        onOk={submitConvert}
        onCancel={() => {
          setConverting(null)
          form.resetFields()
        }}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="code"
            label="主体编码"
            rules={[
              { required: true, message: '请填写编码' },
              { max: 32, message: '最多 32 个字符' },
            ]}
            extra="编码会出现在报价单与发票上，请按公司既有规范填写"
          >
            <Input placeholder="CUS001" />
          </Form.Item>
          <Form.Item name="role" label="角色" initialValue={1}>
            <Select
              options={[
                { value: 1, label: '客户' },
                { value: 2, label: '供应商' },
                { value: 3, label: '客户 + 供应商' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </List>
  )
}
