import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'

import { clauseApi, type ClauseInput } from '@/api/clauses'
import { http } from '@/api/client'
import { Empty, Mono } from '@/components/fields'
import { PageList } from '@/components/page'
import type { Clause, PageResult } from '@/types'
import { formatTime } from '@/utils/format'

/**
 * 条款模板库（T-27）。
 *
 * 这里维护的是**可复用的条款文本**，合同生成时被抄成快照。
 * 别与「付款分期」混了：那个是金额与付款日，这个是"质量异议期 30 天"这类话。
 *
 * 所以改这里的模板，不会改动任何一份已生成的合同——界面上也这么写，
 * 免得有人以为改完模板全公司的合同都跟着变了。
 */
export function ClauseList() {
  const queryClient = useQueryClient()
  // 两个 keyword：输入框里的与已生效的。分开才能只在回车/点查询时请求，
  // 否则每敲一个字母都打一次接口。
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [showDeleted, setShowDeleted] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [editing, setEditing] = useState<Clause | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<ClauseInput>()

  const { data, isLoading, error } = useQuery({
    queryKey: ['clauses', keyword, showDeleted, page, pageSize],
    queryFn: () =>
      http.get<PageResult<Clause>>(
        `/clauses?page=${page}&page_size=${pageSize}` +
          (keyword ? `&keyword=${encodeURIComponent(keyword)}` : '') +
          (showDeleted ? '&include_deleted=true' : ''),
      ),
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['clauses'] })

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setFormOpen(true)
  }

  const openEdit = (row: Clause) => {
    setEditing(row)
    form.setFieldsValue({
      code: row.code,
      title: row.title,
      body: row.body,
      category: row.category,
      remark: row.remark,
    })
    setFormOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      if (editing) {
        await clauseApi.update(editing.id, values)
        message.success('已保存：之后生成的合同用新版本，已生成的合同不受影响')
      } else {
        await clauseApi.create(values)
        message.success('条款已创建')
      }
      setFormOpen(false)
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (row: Clause) => {
    try {
      await clauseApi.remove(row.id)
      message.success('已删除（可恢复）')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const restore = async (row: Clause) => {
    try {
      await clauseApi.restore(row.id)
      message.success('已恢复')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  return (
    <PageList
      title="条款模板"
      description="合同生成时抄成快照；改这里不影响已生成的合同"
      extra={
        <Button type="primary" onClick={openCreate}>
          新建条款
        </Button>
      }
      summary={error ? <Alert type="error" showIcon message={(error as Error).message} /> : null}
      searchFormProps={{
        onFinish: () => {
          setKeyword(keywordInput.trim())
          setPage(1)
        },
      }}
      onReset={() => {
        setKeywordInput('')
        setKeyword('')
        setShowDeleted(false)
        setPage(1)
      }}
      filters={
        <>
          <Form.Item>
            <Input.Search
              placeholder="搜索编码 / 标题 / 正文"
              allowClear
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              onSearch={(v) => {
                setKeyword(v.trim())
                setPage(1)
              }}
            />
          </Form.Item>
          <Form.Item>
            <Select
              value={showDeleted ? 1 : 0}
              onChange={(v) => {
                setShowDeleted(v === 1)
                setPage(1)
              }}
              options={[
                { value: 0, label: '仅未删除' },
                { value: 1, label: '含已删除' },
              ]}
            />
          </Form.Item>
        </>
      }
      tableProps={{
        dataSource: data?.list ?? [],
        loading: isLoading,
        pagination: {
          current: page,
          pageSize,
          total: data?.total ?? 0,
          onChange: (nextPage, nextSize) => {
            setPage(nextPage)
            setPageSize(nextSize)
          },
        },
      }}
      rowKey="id"
      scrollX={1000}
      footer={
        <Modal
          open={formOpen}
          title={editing ? `编辑条款 · ${editing.code}` : '新建条款'}
          onCancel={() => setFormOpen(false)}
          onOk={() => void submit()}
          confirmLoading={saving}
          okText="保存"
          cancelText="返回"
          width={720}
          destroyOnHidden
        >
          <Form form={form} layout="vertical">
            <Form.Item
              name="code"
              label="编码"
              rules={[{ required: true, message: '编码是这块的唯一标识，不能为空' }]}
            >
              <Input placeholder="如 PAY-30 / QUAL-1" disabled={!!editing} />
            </Form.Item>
            <Form.Item name="title" label="标题" rules={[{ required: true, message: '请填写标题' }]}>
              <Input placeholder="如 付款条件 / 质量异议期" />
            </Form.Item>
            <Form.Item name="category" label="分类">
              <Input placeholder="付款 / 质量 / 违约 ……（自由填写）" />
            </Form.Item>
            <Form.Item
              name="body"
              label="正文"
              rules={[{ required: true, message: '条款正文不能为空' }]}
            >
              <Input.TextArea rows={6} placeholder="按合同上的措辞写清楚" />
            </Form.Item>
            <Form.Item name="remark" label="备注">
              <Input placeholder="内部说明，不会印到合同上" />
            </Form.Item>
          </Form>
        </Modal>
      }
    >
      <Table.Column
        dataIndex="code"
        title="编码"
        width={140}
        render={(v: string) => <Mono>{v}</Mono>}
      />
      <Table.Column
        dataIndex="title"
        title="标题"
        width={180}
        render={(v: string) => <span className="cell-strong">{v}</span>}
      />
      <Table.Column
        dataIndex="category"
        title="分类"
        width={100}
        render={(v: string) => (v ? <Tag bordered={false}>{v}</Tag> : <Empty />)}
      />
      <Table.Column
        dataIndex="body"
        title="正文"
        render={(v: string) => (
          <Typography.Text ellipsis style={{ maxWidth: 420 }}>
            {v}
          </Typography.Text>
        )}
      />
      <Table.Column
        dataIndex="updated_at"
        title="更新时间"
        width={170}
        render={(v: string) => formatTime(v)}
      />
      <Table.Column
        title="操作"
        width={150}
        render={(_, row: Clause) => (
          <Space size={4}>
            <Button size="small" type="link" onClick={() => openEdit(row)}>
              编辑
            </Button>
            {row.deleted_at ? (
              <Button size="small" type="link" onClick={() => void restore(row)}>
                恢复
              </Button>
            ) : (
              <Popconfirm
                title="删除这条条款？"
                description="已生成的合同存的是快照，不受影响。"
                okText="删除"
                cancelText="返回"
                onConfirm={() => void remove(row)}
              >
                <Button size="small" type="link" danger>
                  删除
                </Button>
              </Popconfirm>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
