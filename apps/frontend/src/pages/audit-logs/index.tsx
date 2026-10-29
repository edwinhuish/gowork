import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Input, Space, Table, Tag, Tooltip, Typography } from 'antd'

import { auditLogApi } from '@/api/audit-logs'
import { Empty, Mono } from '@/components/fields'
import type { AuditLog } from '@/types'
import { formatTime } from '@/utils/format'

/** 动作的展示色：删除与合并是"不可逆"的那一类，必须显眼。 */
const ACTION_COLOR: Record<string, string> = {
  create: 'green',
  update: 'blue',
  delete: 'red',
  merge: 'volcano',
  status: 'purple',
  restore: 'orange',
}

function actionColor(action: string) {
  return ACTION_COLOR[action]
}

/** 状态码：2xx 中性、4xx 橙、5xx 红。失败的操作同样是"发生过的事"。 */
function statusColor(status: number) {
  if (status >= 500) return 'red'
  if (status >= 400) return 'orange'
  return undefined
}

/**
 * 操作日志（T-33）。
 *
 * 只读：日志不可改，所以这一页没有编辑、没有删除，接口也只有 List。
 * 它回答的是"这张单谁在什么时候改了什么"——`body` 里是**当时提交的内容**，
 * 不是数据库前后 diff（真 diff 要在每个 service 里读旧值，成本高且必然漏）。
 *
 * 只有管理员能看：里面装着全公司单据的提交内容。非管理员访问接口会拿到 403。
 */
export function AuditLogList() {
  const [entityInput, setEntityInput] = useState('')
  const [actionInput, setActionInput] = useState('')
  const [keywordInput, setKeywordInput] = useState('')
  const [fromInput, setFromInput] = useState('')
  const [toInput, setToInput] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [filters, setFilters] = useState({
    entity: '',
    action: '',
    keyword: '',
    from: '',
    to: '',
  })

  const { data, isLoading, error } = useQuery({
    queryKey: ['audit-logs', filters, page, pageSize],
    queryFn: () =>
      auditLogApi.list({
        page,
        page_size: pageSize,
        entity: filters.entity || undefined,
        action: filters.action || undefined,
        keyword: filters.keyword || undefined,
        from: filters.from || undefined,
        to: filters.to || undefined,
      }),
  })

  // 改筛选条件必须回到第一页：停在第 5 页看新条件只会看到空表。
  const apply = () => {
    setFilters({
      entity: entityInput.trim(),
      action: actionInput.trim(),
      keyword: keywordInput.trim(),
      from: fromInput.trim(),
      to: toInput.trim(),
    })
    setPage(1)
  }

  const rows = data?.list ?? []

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Card variant="borderless">
        <Space wrap size={12} align="center">
          <Input
            value={entityInput}
            onChange={(e) => setEntityInput(e.target.value)}
            onPressEnter={apply}
            onBlur={apply}
            placeholder="资源，如 sales-orders"
            style={{ width: 200 }}
            allowClear
            onClear={() => {
              setEntityInput('')
              setFilters((prev) => ({ ...prev, entity: '' }))
              setPage(1)
            }}
          />
          <Input
            value={actionInput}
            onChange={(e) => setActionInput(e.target.value)}
            onPressEnter={apply}
            onBlur={apply}
            placeholder="动作，如 status"
            style={{ width: 150 }}
            allowClear
            onClear={() => {
              setActionInput('')
              setFilters((prev) => ({ ...prev, action: '' }))
              setPage(1)
            }}
          />
          <Input
            value={keywordInput}
            onChange={(e) => setKeywordInput(e.target.value)}
            onPressEnter={apply}
            onBlur={apply}
            placeholder="单号或操作人"
            style={{ width: 200 }}
            allowClear
            onClear={() => {
              setKeywordInput('')
              setFilters((prev) => ({ ...prev, keyword: '' }))
              setPage(1)
            }}
          />
          <Input
            value={fromInput}
            onChange={(e) => setFromInput(e.target.value)}
            onPressEnter={apply}
            onBlur={apply}
            placeholder="起 YYYY-MM-DD"
            style={{ width: 150 }}
            allowClear
            onClear={() => {
              setFromInput('')
              setFilters((prev) => ({ ...prev, from: '' }))
              setPage(1)
            }}
          />
          <Typography.Text type="secondary">至</Typography.Text>
          <Input
            value={toInput}
            onChange={(e) => setToInput(e.target.value)}
            onPressEnter={apply}
            onBlur={apply}
            placeholder="止 YYYY-MM-DD"
            style={{ width: 150 }}
            allowClear
            onClear={() => {
              setToInput('')
              setFilters((prev) => ({ ...prev, to: '' }))
              setPage(1)
            }}
          />
        </Space>
      </Card>

      <Card variant="borderless" title="操作日志">
        <Table<AuditLog>
          dataSource={rows}
          rowKey="id"
          loading={isLoading}
          scroll={{ x: 1300 }}
          locale={{ emptyText: '没有符合条件的操作' }}
          pagination={{
            current: page,
            pageSize,
            total: data?.total ?? 0,
            showSizeChanger: true,
            pageSizeOptions: [20, 50, 100],
            showTotal: (total) => `共 ${total} 条`,
            onChange: (nextPage, nextSize) => {
              setPage(nextPage)
              setPageSize(nextSize)
            },
          }}
        >
          <Table.Column
            dataIndex="created_at"
            title="时间"
            width={180}
            render={(value: string) => formatTime(value)}
          />
          <Table.Column
            title="操作人"
            width={140}
            render={(_, row: AuditLog) => (
              <Space direction="vertical" size={0}>
                <span>{row.actor_name || <Empty />}</span>
                <span className="cell-sub">{row.actor_role}</span>
              </Space>
            )}
          />
          <Table.Column
            dataIndex="action"
            title="动作"
            width={110}
            render={(value: string) => <Tag color={actionColor(value)}>{value}</Tag>}
          />
          <Table.Column
            title="对象"
            width={220}
            render={(_, row: AuditLog) => (
              <Space direction="vertical" size={0}>
                <Mono>{row.entity}</Mono>
                {row.entity_id > 0 && <span className="cell-sub">id {row.entity_id}</span>}
              </Space>
            )}
          />
          <Table.Column
            dataIndex="status"
            title="结果"
            width={80}
            render={(value: number) => <Tag color={statusColor(value)}>{value}</Tag>}
          />
          <Table.Column
            title="提交内容"
            render={(_, row: AuditLog) =>
              row.body ? (
                <Tooltip title={<pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{row.body}</pre>}>
                  <Typography.Text
                    ellipsis
                    style={{ maxWidth: 460, fontFamily: 'monospace', fontSize: 12 }}
                  >
                    {row.body}
                  </Typography.Text>
                </Tooltip>
              ) : (
                <Empty />
              )
            }
          />
          <Table.Column
            dataIndex="path"
            title="路径"
            width={240}
            render={(value: string) => (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {value}
              </Typography.Text>
            )}
          />
          <Table.Column
            dataIndex="request_id"
            title="请求号"
            width={140}
            render={(value: string) => (value ? <Mono>{value}</Mono> : <Empty />)}
          />
        </Table>

        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          记录的是<b>当时提交的内容</b>（已脱敏、超长截断），不是数据库前后 diff；
          失败的操作（4xx / 5xx）同样留痕。日志只增不改：本页没有编辑与删除，
          后端的仓储也只有写入与查询两个方法。
        </Typography.Paragraph>
      </Card>
    </Space>
  )
}
