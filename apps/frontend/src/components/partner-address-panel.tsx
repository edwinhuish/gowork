import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'

import { partnerApi, type PartnerAddressInput } from '@/api/partners'
import { PARTNER_ADDRESS_TYPE_OPTIONS } from '@/constants/catalog'
import type { PartnerAddress } from '@/types'

const TYPE_LABEL = new Map(PARTNER_ADDRESS_TYPE_OPTIONS.map((o) => [o.value, o.label]))

/**
 * 主体详情页的多地址面板（T-49）。
 *
 * 一个主体可以有多条地址，按用途分成四类：发票抬头、收货地址、提单收货人、通知方。
 * 单证取址时就靠这个分类——外贸一张 CI 上三栏经常指向三家公司，
 * 所以这里不是"备用联系方式"，而是单证的取数依据。
 *
 * 默认地址的规则由后端保证：该用途第一条自动默认、设新的会把旧的降级、
 * 取消唯一一条默认会被拒绝。界面只负责把结果展示出来，不重复实现这套判断。
 */
export function PartnerAddressPanel({ partnerId }: { partnerId: number }) {
  const [rows, setRows] = useState<PartnerAddress[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<PartnerAddress | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<PartnerAddressInput>()

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      setRows(await partnerApi.addresses.list(partnerId))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '地址加载失败')
    } finally {
      setLoading(false)
    }
  }, [partnerId])

  useEffect(() => {
    void reload()
  }, [reload])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ type: 'invoice', is_default: false })
    setOpen(true)
  }

  const openEdit = (record: PartnerAddress) => {
    setEditing(record)
    form.setFieldsValue({
      type: record.type,
      is_default: record.is_default,
      contact: record.contact,
      phone: record.phone,
      country: record.country,
      province: record.province,
      city: record.city,
      postal_code: record.postal_code,
      address: record.address,
      remark: record.remark,
    })
    setOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      if (editing) {
        await partnerApi.addresses.update(partnerId, editing.id, values)
      } else {
        await partnerApi.addresses.create(partnerId, values)
      }
      message.success(editing ? '地址已更新' : '地址已新增')
      setOpen(false)
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (record: PartnerAddress) => {
    try {
      await partnerApi.addresses.remove(partnerId, record.id)
      message.success('地址已删除')
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      title="地址"
      size="small"
      extra={
        <Button size="small" onClick={openCreate}>
          新增地址
        </Button>
      }
      style={{ marginTop: 16 }}
    >
      <Table<PartnerAddress>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: '还没有地址。单证会自动回退到主体上的地址。' }}
        columns={[
          {
            title: '用途',
            dataIndex: 'type',
            width: 120,
            render: (value: PartnerAddress['type']) => (
              <Tag bordered={false}>{TYPE_LABEL.get(value) ?? value}</Tag>
            ),
          },
          {
            title: '地址',
            dataIndex: 'address',
            render: (_: string, record) => {
              const parts = [record.country, record.province, record.city, record.address]
                .filter(Boolean)
                .join(', ')
              return (
                <Space direction="vertical" size={0}>
                  <span>{parts || '—'}</span>
                  {record.remark ? <span className="cell-sub">{record.remark}</span> : null}
                </Space>
              )
            },
          },
          {
            title: '联系人',
            dataIndex: 'contact',
            width: 160,
            render: (_: string, record) =>
              record.contact || record.phone ? (
                <Space direction="vertical" size={0}>
                  <span>{record.contact || '—'}</span>
                  {record.phone ? <span className="cell-sub">{record.phone}</span> : null}
                </Space>
              ) : (
                <span className="cell-sub">—</span>
              ),
          },
          {
            title: '默认',
            dataIndex: 'is_default',
            width: 70,
            // 只读展示：切换默认请用编辑弹窗，后端要在同一事务里降级原默认。
            render: (value: boolean) =>
              value ? <Tag color="blue">默认</Tag> : <span className="cell-sub">—</span>,
          },
          {
            title: '操作',
            width: 120,
            render: (_: unknown, record) => (
              <Space size={4}>
                <Button size="small" type="link" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="删除这条地址？"
                  description={
                    record.is_default
                      ? '删掉后，同用途剩下最早的一条会自动接替为默认。'
                      : '删除后不影响其它地址。'
                  }
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
        title={editing ? '编辑地址' : '新增地址'}
        width={720}
        onCancel={() => setOpen(false)}
        onOk={() => void submit()}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical">
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', columnGap: 16 }}>
            <Form.Item
              name="type"
              label="用途"
              rules={[{ required: true, message: '请选择用途' }]}
              tooltip="改用途等于这条地址换了角色，建议删掉重建；否则原用途会失去默认"
            >
              {/* 编辑时禁用：改类型会让原用途可能没有默认，而默认由后端事务维护，
                  前端就地改类型绕过了那套判断。删了重建是最省事的正确做法。 */}
              <Select options={PARTNER_ADDRESS_TYPE_OPTIONS} disabled={!!editing} />
            </Form.Item>

            <Form.Item
              name="is_default"
              label="设为默认"
              valuePropName="checked"
              tooltip="每个用途最多一条默认；设为默认会把同用途原来的那条降级"
            >
              <Switch />
            </Form.Item>

            <Form.Item name="contact" label="联系人" rules={[{ max: 64, message: '最多 64 个字符' }]}>
              <Input />
            </Form.Item>

            <Form.Item name="phone" label="电话" rules={[{ max: 32, message: '最多 32 个字符' }]}>
              <Input />
            </Form.Item>

            <Form.Item name="country" label="国家/地区" rules={[{ max: 64, message: '最多 64 个字符' }]}>
              <Input placeholder="厄瓜多尔" />
            </Form.Item>

            <Form.Item name="province" label="省/州" rules={[{ max: 64, message: '最多 64 个字符' }]}>
              <Input />
            </Form.Item>

            <Form.Item name="city" label="城市" rules={[{ max: 64, message: '最多 64 个字符' }]}>
              <Input />
            </Form.Item>

            <Form.Item name="postal_code" label="邮编" rules={[{ max: 16, message: '最多 16 个字符' }]}>
              <Input />
            </Form.Item>
          </div>

          <Form.Item
            name="address"
            label="详细地址"
            rules={[
              { required: true, message: '请填写详细地址' },
              { max: 255, message: '最多 255 个字符' },
            ]}
            tooltip="门牌与街道即可，国家省市在上面分开填"
          >
            <Input.TextArea rows={2} />
          </Form.Item>

          <Form.Item name="remark" label="备注" rules={[{ max: 255, message: '最多 255 个字符' }]}>
            <Input placeholder="如「仅工作日收货」" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
