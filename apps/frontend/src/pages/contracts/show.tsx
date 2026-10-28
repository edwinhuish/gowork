import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useInvalidate, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import { Button, Descriptions, Divider, message, Popconfirm, Space, Table, Tag } from 'antd'

import { contractApi } from '@/api/contracts'
import { Empty, Mono, Money } from '@/components/fields'
import {
  contractSourceTypeLabel,
  contractStatusLabel,
  contractTypeLabel,
} from '@/constants/catalog'
import type { Contract, ContractTerm } from '@/types'
import { formatDate } from '@/utils/format'

function statusColor(value: number): string {
  if (value === 20) return 'success'
  if (value === 90) return 'default'
  return 'processing'
}

function typeColor(value: number): string {
  return value === 10 ? 'blue' : 'geekblue'
}

/**
 * 合同详情。
 *
 * 只读：金额、汇率与分期都是生成时锁定的快照。这里能做的只有两件事——
 * 把草稿确认为承诺，或取消它；要改条款请回编辑页（仅草稿）。
 */
export function ContractShow() {
  const { result, query } = useShow<Contract>({ resource: 'contracts' })
  const invalidate = useInvalidate()
  const navigate = useNavigate()
  const [acting, setActing] = useState(false)

  const c = result
  const terms = c?.terms ?? []
  const termsTotal = terms.reduce((acc, t) => acc + t.amount, 0)
  const termsBaseTotal = terms.reduce((acc, t) => acc + t.amount_base, 0)
  const matched =
    c !== undefined && Math.abs(termsTotal - c.amount_total) < 0.005

  const refresh = () => invalidate({ resource: 'contracts', invalidates: ['detail', 'list'] })

  const changeStatus = async (status: number, okText: string) => {
    if (!c) return
    setActing(true)
    try {
      await contractApi.updateStatus(c.id, status)
      message.success(okText)
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActing(false)
    }
  }

  const openSource = () => {
    if (!c) return
    const path =
      c.source_type === 20
        ? `/sales-orders/show/${c.source_id}`
        : `/quotations/show/${c.source_id}`
    navigate(path)
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={c ? `${c.doc_no} · ${contractTypeLabel(c.type)}` : '合同详情'}
      headerButtons={
        <Space wrap>
          {c?.status === 10 && <EditButton resource="contracts" recordItemId={c.id} />}
          {c?.status === 10 && (
            <Popconfirm
              title="确认这份合同？"
              description="确认后金额与分期锁定、不可修改，只能取消"
              okText="确认"
              cancelText="再想想"
              onConfirm={() => changeStatus(20, '合同已确认')}
            >
              <Button type="primary" loading={acting}>
                确认合同
              </Button>
            </Popconfirm>
          )}
          {(c?.status === 10 || c?.status === 20) && (
            <Popconfirm
              title="取消这份合同？"
              description="取消是终态，记录保留供追溯，但不再参与统计"
              okText="取消合同"
              cancelText="返回"
              onConfirm={() => changeStatus(90, '合同已取消')}
            >
              <Button danger loading={acting}>
                取消合同
              </Button>
            </Popconfirm>
          )}
        </Space>
      }
    >
      <Descriptions column={2} bordered size="small">
        <Descriptions.Item label="合同类型">
          <Tag color={c ? typeColor(c.type) : undefined}>{contractTypeLabel(c?.type)}</Tag>
        </Descriptions.Item>
        <Descriptions.Item label="状态">
          <Tag color={c ? statusColor(c.status) : undefined}>{contractStatusLabel(c?.status)}</Tag>
        </Descriptions.Item>
        <Descriptions.Item label="客户">{c?.partner?.name || <Empty />}</Descriptions.Item>
        <Descriptions.Item label="来源单据">
          {c ? (
            <Space size={6}>
              <Tag>{contractSourceTypeLabel(c.source_type)}</Tag>
              <Button type="link" size="small" style={{ padding: 0 }} onClick={openSource}>
                {c.source_doc_no}
              </Button>
            </Space>
          ) : (
            <Empty />
          )}
        </Descriptions.Item>
        <Descriptions.Item label="签订日">{formatDate(c?.sign_date)}</Descriptions.Item>
        <Descriptions.Item label="约定交期">
          {formatDate(c?.delivery_date ?? undefined)}
        </Descriptions.Item>
        <Descriptions.Item label="币种 / 汇率">
          {c ? (
            <Space size={6}>
              <Tag>{c.currency_code}</Tag>
              <span>{c.exchange_rate}</span>
            </Space>
          ) : (
            <Empty />
          )}
        </Descriptions.Item>
        <Descriptions.Item label="原币总额">
          {c ? (
            <Space size={4}>
              <Money value={c.amount_total} />
              <Tag>{c.currency_code}</Tag>
            </Space>
          ) : (
            <Empty />
          )}
        </Descriptions.Item>
        <Descriptions.Item label="折本币">
          {c ? <Money value={c.amount_base_total} /> : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="分期合计">
          {terms.length === 0 ? (
            <Empty />
          ) : (
            <span style={{ color: matched ? undefined : '#cf1322' }}>
              <Money value={termsTotal} />
              {matched ? '' : '（与总额不一致）'}
            </span>
          )}
        </Descriptions.Item>
        <Descriptions.Item label="备注" span={2}>
          {c?.remark || <Empty />}
        </Descriptions.Item>
      </Descriptions>

      <Divider orientation="left" style={{ margin: '24px 0 12px' }}>
        付款分期
      </Divider>

      <Table<ContractTerm>
        rowKey="id"
        size="small"
        dataSource={terms}
        pagination={false}
        summary={() =>
          terms.length === 0 ? null : (
            <Table.Summary.Row>
              <Table.Summary.Cell index={0} colSpan={2}>
                合计
              </Table.Summary.Cell>
              <Table.Summary.Cell index={1} align="right">
                <Money value={termsTotal} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={2} align="right">
                <Money value={termsBaseTotal} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={3} colSpan={2} />
            </Table.Summary.Row>
          )
        }
      >
        <Table.Column dataIndex="seq" title="#" width={56} />
        <Table.Column dataIndex="name" title="期次" width={180} />
        <Table.Column
          dataIndex="amount"
          title="金额"
          width={150}
          align="right"
          render={(value: number) => (
            <Space size={4}>
              <Money value={value} />
              <Tag>{c?.currency_code}</Tag>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="amount_base"
          title="折本币"
          width={140}
          align="right"
          render={(value: number) => <Money value={value} />}
        />
        <Table.Column
          dataIndex="due_date"
          title="付款日"
          width={120}
          render={(value: string | null) => formatDate(value ?? undefined)}
        />
        <Table.Column
          dataIndex="trigger"
          title="付款条件"
          render={(value: string) => value || <Empty />}
        />
      </Table>

      <div style={{ marginTop: 12, color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
        合同不存货物明细：明细留在来源单据上，这里只有一份对外承诺的快照。
        单号 <Mono>{c?.doc_no ?? ''}</Mono> 本身能看出是 PI 还是销售合同。
      </div>
    </Show>
  )
}
