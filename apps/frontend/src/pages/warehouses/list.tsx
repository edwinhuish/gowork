import { useState } from 'react'
import type { CrudFilters } from '@refinedev/core'
import { useList } from '@refinedev/core'
import { Button, Checkbox, Form, Input, Modal, Popconfirm, Space, Switch, Table, Tag, message } from 'antd'

import { http } from '@/api/client'
import { Empty } from '@/components/fields'
import { PageList } from '@/components/page'
import type { Warehouse } from '@/types'

interface WarehouseForm {
  code: string
  name: string
  address?: string
  is_default: boolean
  remark?: string
}

/**
 * 仓库维护（T-56）。
 *
 * 在此之前仓库只有一条启动时自动建的「主仓」，界面上一处都改不了，
 * 而调拨、按仓盘点、按仓看库存都要先有两个仓可选。
 *
 * 删除是软删：被流水引用过的仓删掉之后，历史库存仍指向它，
 * 只是不再出现在下拉里，需要时能原样恢复回去。默认仓不让删——
 * 它不是"更常用的一条"，而是新流水唯一能自动落的位置。
 */
export function WarehouseList() {
  const [showDeleted, setShowDeleted] = useState(false)
  const { query, result } = useList<Warehouse>({
    resource: 'warehouses',
    pagination: { mode: 'off' },
    filters: showDeleted
      ? ([{ field: 'include_deleted', operator: 'eq', value: true }] as CrudFilters)
      : [],
  })
  const rows: Warehouse[] = result?.data ?? []

  const [editing, setEditing] = useState<Warehouse | null>(null)
  const [creating, setCreating] = useState(false)
  const [form] = Form.useForm<WarehouseForm>()
  const [saving, setSaving] = useState(false)

  const reload = () => query.refetch()

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ is_default: false })
    setCreating(true)
  }

  const openEdit = (row: Warehouse) => {
    form.setFieldsValue({
      code: row.code,
      name: row.name,
      address: row.address,
      is_default: row.is_default,
      remark: row.remark,
    })
    setEditing(row)
  }

  const remove = async (row: Warehouse) => {
    try {
      await http.delete<{ deleted: boolean }>(`/warehouses/${row.id}`)
      message.success('已删除仓库')
      await reload()
    } catch (err) {
      // 默认仓不让删是 409，后端文案写明了要先换个默认仓。
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const restore = async (row: Warehouse) => {
    try {
      await http.put<Warehouse>(`/warehouses/${row.id}/restore`)
      message.success('已恢复仓库')
      await reload()
    } catch (err) {
      // 编码被别的仓占了是 409，后端文案写明了是哪个编码。
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
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
      description="只有默认仓会被新流水自动使用；删除是软删，必要时可以恢复回来"
      extra={
        <Space size={8}>
          <Checkbox checked={showDeleted} onChange={(e) => setShowDeleted(e.target.checked)}>
            含已删除
          </Checkbox>
          <Button type="primary" onClick={openCreate}>
            新增仓库
          </Button>
        </Space>
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
        title="操作"
        width={140}
        render={(_: unknown, row: Warehouse) => (
          <Space size={4}>
            {row.deleted_at ? (
              // 已软删的仓只能恢复：它的编码可能已经被别人占了，
              // 那时后端 409 会说明是哪个编码。
              <Button size="small" type="link" onClick={() => restore(row)}>
                恢复
              </Button>
            ) : (
              <>
                <Button size="small" type="link" onClick={() => openEdit(row)}>
                  编辑
                </Button>
                <Popconfirm
                  title={`删除仓库 ${row.code}？`}
                  description="默认仓不能删除；删掉的仓不再出现在下拉里，可恢复"
                  okText="删除"
                  cancelText="取消"
                  onConfirm={() => remove(row)}
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
    </PageList>
  )
}
