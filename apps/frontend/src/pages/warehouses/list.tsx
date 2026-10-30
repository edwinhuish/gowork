import { useState } from 'react'
import { useList } from '@refinedev/core'
import { Button, Form, Input, Modal, Switch, Table, Tag, message } from 'antd'

import { http } from '@/api/client'
import { Empty, StatusTag } from '@/components/fields'
import { PageList } from '@/components/page'
import type { Warehouse } from '@/types'

interface WarehouseForm {
  code: string
  name: string
  address?: string
  is_default: boolean
  status: boolean
  remark?: string
}

/**
 * 仓库维护（T-56）。
 *
 * 在此之前仓库只有一条启动时自动建的「主仓」，界面上一处都改不了，
 * 而调拨、按仓盘点、按仓看库存都要先有两个仓可选。
 *
 * 只做三件事：列出来、建一个、设默认。**不提供删除**：
 * 仓库被流水引用之后删掉，历史库存会指向一个不存在的仓——
 * 不要了就把状态关掉，它只是不再出现在下拉里。
 */
export function WarehouseList() {
  const { query, result } = useList<Warehouse>({
    resource: 'warehouses',
    pagination: { mode: 'off' },
  })
  const rows: Warehouse[] = result?.data ?? []

  const [editing, setEditing] = useState<Warehouse | null>(null)
  const [creating, setCreating] = useState(false)
  const [form] = Form.useForm<WarehouseForm>()
  const [saving, setSaving] = useState(false)

  const reload = () => query.refetch()

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ is_default: false, status: true })
    setCreating(true)
  }

  const openEdit = (row: Warehouse) => {
    form.setFieldsValue({
      code: row.code,
      name: row.name,
      address: row.address,
      is_default: row.is_default,
      status: row.status === 1,
      remark: row.remark,
    })
    setEditing(row)
  }

  const submit = async () => {
    const v = await form.validateFields()
    setSaving(true)
    try {
      const body = {
        code: v.code,
        name: v.name,
        address: v.address ?? '',
        is_default: !!v.is_default,
        status: v.status ? 1 : 0,
        remark: v.remark ?? '',
      }
      if (editing) {
        await http.put(`/warehouses/${editing.id}`, body)
        message.success('仓库已更新')
      } else {
        await http.post('/warehouses', body)
        message.success('仓库已创建')
      }
      setEditing(null)
      setCreating(false)
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <PageList
      title="仓库"
      description="只有默认仓会被新流水自动使用；停用的仓不出现在下拉里"
      extra={
        <Button type="primary" onClick={openCreate}>
          新增仓库
        </Button>
      }
      tableProps={{ dataSource: rows, loading: query.isLoading, pagination: false }}
      rowKey="id"
      footer={
        <Modal
          open={creating || !!editing}
          title={editing ? `编辑仓库 · ${editing.code}` : '新增仓库'}
          onCancel={() => {
            setEditing(null)
            setCreating(false)
          }}
          onOk={() => void submit()}
          confirmLoading={saving}
          okText="保存"
          cancelText="取消"
          forceRender
        >
          <Form form={form} layout="vertical">
            <Form.Item
              name="code"
              label="编码"
              rules={[{ required: true, message: '请填编码' }]}
              extra={editing ? '编码不可修改：历史流水与报表认的是它' : '如 MAIN / SZ / NB'}
            >
              <Input disabled={!!editing} maxLength={32} placeholder="MAIN" />
            </Form.Item>
            <Form.Item name="name" label="名称" rules={[{ required: true, message: '请填名称' }]}>
              <Input maxLength={64} placeholder="主仓" />
            </Form.Item>
            <Form.Item name="address" label="地址">
              <Input maxLength={255} placeholder="印在出入库单上" />
            </Form.Item>
            <Form.Item
              name="is_default"
              label="设为默认仓"
              valuePropName="checked"
              extra="设为默认会把其它仓的默认取消：允许多条默认，等于每次随机挑一个"
            >
              <Switch />
            </Form.Item>
            <Form.Item name="status" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="remark" label="备注">
              <Input.TextArea rows={2} maxLength={255} />
            </Form.Item>
          </Form>
        </Modal>
      }
    >
      <Table.Column dataIndex="code" title="编码" width={120} />
      <Table.Column dataIndex="name" title="名称" width={180} />
      <Table.Column
        dataIndex="address"
        title="地址"
        render={(v: string) => v || <Empty />}
      />
      <Table.Column
        dataIndex="is_default"
        title="默认"
        width={90}
        render={(v: boolean) => (v ? <Tag color="blue">默认</Tag> : <Empty />)}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={90}
        render={(v: number) => <StatusTag value={v} />}
      />
      <Table.Column
        title="操作"
        width={90}
        render={(_: unknown, row: Warehouse) => (
          <Button size="small" type="link" onClick={() => openEdit(row)}>
            编辑
          </Button>
        )}
      />
    </PageList>
  )
}
