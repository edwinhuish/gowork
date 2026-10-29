import { useCallback, useEffect, useState } from 'react'
import { List } from '@refinedev/antd'
import { Button, Form, Input, Modal, Select, Space, Switch, Table, Tag, message } from 'antd'

import { userAccountApi, type UserUpdateInput } from '@/api/user-accounts'
import type { SystemUser } from '@/types'

/** 与后端 model.RoleAdmin / RoleManager / RoleStaff 一致。 */
const ROLE_OPTIONS = [
  { value: 'admin', label: '管理员' },
  { value: 'manager', label: '主管' },
  { value: 'staff', label: '业务员' },
]

const ROLE_LABEL = new Map(ROLE_OPTIONS.map((o) => [o.value, o.label]))

interface FormValues {
  username: string
  password?: string
  nickname?: string
  role: string
  enabled?: boolean
}

/**
 * 账号维护（T-57）。
 *
 * 之前建号、改角色、停用、重置密码都只能改数据库，而角色写在 JWT 里，
 * 改完还要等令牌过期才生效——「离职当天还能登进来」是这类系统最不该有的行为。
 * 现在每次变更都会吊销该账号的会话，他下一次请求就会拿到 401。
 */
export function UserAccountList() {
  const [rows, setRows] = useState<SystemUser[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<SystemUser | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<FormValues>()

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      setRows(await userAccountApi.list())
    } catch (err) {
      // 业务员点进来会拿到 403：菜单对所有人可见，权限由接口兜底。
      message.error(err instanceof Error ? err.message : '账号列表加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ role: 'staff', enabled: true })
    setOpen(true)
  }

  const openEdit = (record: SystemUser) => {
    setEditing(record)
    form.resetFields()
    form.setFieldsValue({
      username: record.username,
      nickname: record.nickname,
      role: record.role,
      enabled: record.status === 1,
      password: '',
    })
    setOpen(true)
  }

  const submit = async () => {
    const v = await form.validateFields()
    setSaving(true)
    try {
      if (editing) {
        // 只提交真的改了的字段：undefined 在后端表示"这次不改"，
        // 传空串会把昵称清空、传 false 会把账号停用。
        const patch: UserUpdateInput = {}
        if ((v.nickname ?? '') !== editing.nickname) patch.nickname = v.nickname ?? ''
        if (v.role !== editing.role) patch.role = v.role
        const nextStatus = v.enabled ? 1 : 0
        if (nextStatus !== editing.status) patch.status = nextStatus
        if (v.password) patch.password = v.password
        if (Object.keys(patch).length === 0) {
          message.info('没有要保存的改动')
          setOpen(false)
          return
        }
        await userAccountApi.update(editing.id, patch)
        message.success('账号已更新；若改了角色或状态，该成员的登录状态已失效')
      } else {
        await userAccountApi.create({
          username: v.username,
          password: v.password ?? '',
          nickname: v.nickname,
          role: v.role,
        })
        message.success('账号已创建')
      }
      setOpen(false)
      await reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <List title="账号" createButtonProps={undefined}>
      <div style={{ marginBottom: 12 }}>
        <Space>
          <Button type="primary" onClick={openCreate}>
            新建账号
          </Button>
          <span className="cell-sub">
            改角色、停用、重置密码都会让该账号已登录的会话立即失效
          </span>
        </Space>
      </div>

      <Table<SystemUser>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={rows}
        pagination={false}
        columns={[
          { title: '用户名', dataIndex: 'username', width: 180 },
          {
            title: '昵称',
            dataIndex: 'nickname',
            render: (v: string) => v || <span className="cell-sub">—</span>,
          },
          {
            title: '角色',
            dataIndex: 'role',
            width: 110,
            // 管理员用主色标出来：改角色是最容易点错的一列。
            render: (v: string) => (
              <Tag bordered={false} color={v === 'admin' ? 'blue' : undefined}>
                {ROLE_LABEL.get(v) ?? v}
              </Tag>
            ),
          },
          {
            title: '状态',
            dataIndex: 'status',
            width: 90,
            render: (v: number) =>
              v === 1 ? <Tag color="green">启用</Tag> : <Tag color="default">停用</Tag>,
          },
          {
            title: '操作',
            width: 100,
            render: (_: unknown, record) => (
              <Button size="small" type="link" onClick={() => openEdit(record)}>
                编辑
              </Button>
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title={editing ? `编辑账号 ${editing.username}` : '新建账号'}
        onCancel={() => setOpen(false)}
        onOk={() => void submit()}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        forceRender
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请填写用户名' }, { max: 64, message: '最多 64 个字符' }]}
            tooltip="登录凭据，建号之后不能改——改了等于换了个人，历史操作记录会对不上"
          >
            <Input disabled={!!editing} placeholder="alice" />
          </Form.Item>

          <Form.Item
            name="nickname"
            label="昵称"
            rules={[{ max: 64, message: '最多 64 个字符' }]}
          >
            <Input placeholder="爱丽丝" />
          </Form.Item>

          <Form.Item
            name="role"
            label="角色"
            rules={[{ required: true, message: '请选择角色' }]}
            tooltip="管理员能改系统设置，主管能看全部数据，业务员只看自己负责的客户"
          >
            <Select options={ROLE_OPTIONS} />
          </Form.Item>

          {editing ? (
            <>
              <Form.Item
                name="enabled"
                label="启用"
                valuePropName="checked"
                tooltip="停用后该账号立即无法登录，已登录的会话也会失效"
              >
                <Switch />
              </Form.Item>
              <Form.Item
                name="password"
                label="重置密码"
                rules={[{ min: 6, message: '至少 6 位' }, { max: 72, message: '最多 72 个字符' }]}
                tooltip="留空表示不改。改了之后该成员需要重新登录"
              >
                <Input.Password placeholder="留空不修改" autoComplete="new-password" />
              </Form.Item>
            </>
          ) : (
            <Form.Item
              name="password"
              label="初始密码"
              rules={[
                { required: true, message: '请设置初始密码' },
                { min: 6, message: '至少 6 位' },
                { max: 72, message: '最多 72 个字符' },
              ]}
              tooltip="至少 6 位。建好之后请让对方自行修改"
            >
              <Input.Password placeholder="至少 6 位" autoComplete="new-password" />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </List>
  )
}
