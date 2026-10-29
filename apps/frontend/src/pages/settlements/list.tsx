import { useInvalidate } from '@refinedev/core'
import { CreateButton, EditButton, List, useTable } from '@refinedev/antd'
import { Button, message, Popconfirm, Space, Table, Tag } from 'antd'

import { receivableApi } from '@/api/receivables'
import { payableApi } from '@/api/payables'
import { Empty, Mono, Money } from '@/components/fields'
import { settlementStatusColor, settlementStatusLabel } from '@/constants/catalog'
import type { Settlement } from '@/types'
import { formatDate } from '@/utils/format'

import type { SettlementDirection } from './form-fields'

/**
 * 应收 / 应付列表。两个模块共用，差别只在资源名与文案。
 *
 * 默认按到期日升序（后端排的）：这张表的读法是"哪笔先到期"，
 * 不是"我刚录了什么"。过期未结清的行把日期标红。
 */
export function SettlementList({
  resource,
  direction,
}: {
  resource: 'receivables' | 'payables'
  direction: SettlementDirection
}) {
  const isReceivable = direction === 'receivable'
  const invalidate = useInvalidate()
  const { tableProps } = useTable<Settlement>({ resource, syncWithLocation: true })

  const api = isReceivable ? receivableApi : payableApi

  const voidOne = async (record: Settlement) => {
    try {
      // 作废要带来源字段（后端要求必填），但来源本身不会变。
      await api.update(record.id, {
        source_type: record.source_type,
        source_id: record.source_id,
        status: 90,
        remark: record.remark,
      })
      message.success(`${record.doc_no} 已作废`)
      await invalidate({ resource, invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '作废失败')
    }
  }

  const remove = async (record: Settlement) => {
    try {
      await api.remove(record.id)
      message.success(`已删除 ${record.doc_no}`)
      await invalidate({ resource, invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const today = new Date().toISOString().slice(0, 10)

  return (
    <List
      headerButtons={
        <CreateButton>{isReceivable ? '新建应收' : '新建应付'}</CreateButton>
      }
    >
      <Table {...tableProps} rowKey="id" scroll={{ x: 1100 }}>
        <Table.Column
          dataIndex="doc_no"
          title="单号"
          width={160}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="partner"
          title={isReceivable ? '客户' : '供应商'}
          width={180}
          render={(value: Settlement['partner']) =>
            value ? <span className="cell-strong">{value.name}</span> : <Empty />
          }
        />
        <Table.Column
          dataIndex="source_doc_no"
          title="来源单据"
          width={150}
          render={(value: string) => (value ? <Mono>{value}</Mono> : <Empty />)}
        />
        <Table.Column
          dataIndex="amount"
          title="金额"
          width={160}
          align="right"
          render={(value: string, record: Settlement) => (
            <Space size={6}>
              <Money value={value} />
              <Tag>{record.currency_code}</Tag>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="settled_amount"
          title="已结清"
          width={120}
          align="right"
          render={(value: string) =>
            Number(value) > 0 ? <Money value={value} /> : <Empty />
          }
        />
        <Table.Column
          title="未结清"
          width={120}
          align="right"
          // 金额是定点字符串：未结清只是显示用的差，真正的余额在服务端按末位算。
          render={(_, record: Settlement) =>
            record.status === 90 ? (
              <Empty />
            ) : (
              <Money value={Number(record.amount) - Number(record.settled_amount)} />
            )
          }
        />
        <Table.Column
          dataIndex="due_date"
          title="到期日"
          width={120}
          render={(value: string, record: Settlement) => {
            const overdue = record.status !== 30 && record.status !== 90 && value.slice(0, 10) < today
            return (
              <span style={overdue ? { color: '#cf1322' } : undefined}>
                {formatDate(value)}
                {overdue && <Tag color="red" style={{ marginLeft: 6 }}>逾期</Tag>}
              </span>
            )
          }}
        />
        <Table.Column
          dataIndex="status"
          title="状态"
          width={110}
          render={(value: number) => (
            <Tag color={settlementStatusColor(value)}>{settlementStatusLabel(value)}</Tag>
          )}
        />
        <Table.Column
          title="操作"
          width={200}
          render={(_, record: Settlement) => (
            <Space size={4}>
              <EditButton size="small" type="link" recordItemId={record.id}>
                编辑
              </EditButton>
              {record.status !== 90 && (
                <Popconfirm
                  title="作废这张单？"
                  description="作废后不再占用来源单据的开票额度，单据本身保留"
                  okText="作废"
                  cancelText="取消"
                  onConfirm={() => voidOne(record)}
                >
                  <Button size="small" type="link">
                    作废
                  </Button>
                </Popconfirm>
              )}
              <Popconfirm
                title="删除这张单？"
                description="收过款的单不能删，请改用「作废」"
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
    </List>
  )
}
