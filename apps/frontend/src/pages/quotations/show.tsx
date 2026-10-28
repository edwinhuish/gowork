import { useState } from 'react'
import { useInvalidate, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Input,
  message,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
} from 'antd'

import { quotationApi } from '@/api/quotations'
import { Empty, Mono, Money } from '@/components/fields'
import { quotationStatusLabel, tradeTermLabel } from '@/constants/catalog'
import type { QuotationDetail, QuotationItem } from '@/types'
import { formatDate, formatPercent, formatTime } from '@/utils/format'

import { defaultValidUntil } from './form-fields'

function statusColor(value: number): string {
  if (value === 20) return 'success'
  if (value === 90) return 'default'
  return 'processing'
}

/**
 * 报价单详情。
 *
 * 三件事只在这里做：确认（把报价变成承诺）、作废、另存新版本。
 * 明细与合计全部只读——金额是保存时锁定的快照，页面上没有可以改它的入口。
 */
export function QuotationShow() {
  const { result, query } = useShow<QuotationDetail>({ resource: 'quotations' })
  const invalidate = useInvalidate()
  const [acting, setActing] = useState(false)
  const [versionOpen, setVersionOpen] = useState(false)
  const [newValidUntil, setNewValidUntil] = useState('')

  const q = result
  const refresh = () => invalidate({ resource: 'quotations', invalidates: ['detail', 'list'] })

  const changeStatus = async (status: number, okText: string) => {
    if (!q) return
    setActing(true)
    try {
      await quotationApi.updateStatus(q.id, status)
      message.success(okText)
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActing(false)
    }
  }

  const saveVersion = async () => {
    if (!q) return
    setActing(true)
    try {
      const v = await quotationApi.newVersion(q.id, newValidUntil || undefined)
      message.success(`已另存为第 ${v.version} 版`)
      setVersionOpen(false)
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '另存失败')
    } finally {
      setActing(false)
    }
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={q ? `${q.doc_no} · v${q.version}` : '报价单详情'}
      headerButtons={
        <Space wrap>
          {q?.status === 10 && <EditButton resource="quotations" recordItemId={q.id} />}
          {q?.status === 10 && (
            <Button
              type="primary"
              loading={acting}
              onClick={() => changeStatus(20, '报价单已确认')}
            >
              确认报价
            </Button>
          )}
          {(q?.status === 10 || q?.status === 20) && (
            <Popconfirm
              title="作废这张报价单？"
              description="作废是终态，不能再确认；需要重新报价请另存新版本"
              okText="作废"
              cancelText="取消"
              onConfirm={() => changeStatus(90, '报价单已作废')}
            >
              <Button danger>作废</Button>
            </Popconfirm>
          )}
          <Button
            onClick={() => {
              setNewValidUntil(q?.expired ? defaultValidUntil() : '')
              setVersionOpen(true)
            }}
          >
            另存新版本
          </Button>
        </Space>
      }
    >
      {q?.convert_block && (
        <Alert
          type={q.expired ? 'error' : 'warning'}
          showIcon
          style={{ marginBottom: 16 }}
          message={`不可转订单：${q.convert_block}`}
        />
      )}

      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="middle"
        items={[
          { key: 'doc_no', label: '单号', children: <span className="cell-mono">{q?.doc_no}</span> },
          { key: 'version', label: '版本', children: q ? `v${q.version}` : <Empty /> },
          {
            key: 'status',
            label: '状态',
            children: q ? (
              <Tag color={statusColor(q.status)}>{quotationStatusLabel(q.status)}</Tag>
            ) : (
              <Empty />
            ),
          },
          { key: 'partner', label: '客户', children: q?.partner?.name || <Empty /> },
          {
            key: 'trade_term',
            label: '贸易术语',
            children: q?.trade_term ? <Tag>{tradeTermLabel(q.trade_term)}</Tag> : <Empty />,
          },
          {
            key: 'valid_until',
            label: '有效期至',
            children: q ? (
              <Space size={6}>
                {formatDate(q.valid_until)}
                {q.expired && <Tag color="error">已过期</Tag>}
              </Space>
            ) : (
              <Empty />
            ),
          },
          {
            key: 'currency_code',
            label: '报价币种 / 汇率',
            children: q ? (
              <Space size={6}>
                <Tag>{q.currency_code}</Tag>
                <span className="cell-mono">{q.exchange_rate}</span>
              </Space>
            ) : (
              <Empty />
            ),
          },
          {
            key: 'amount_total',
            label: '报价总额',
            children: q ? <Money value={q.amount_total} /> : <Empty />,
          },
          {
            key: 'amount_base_total',
            label: '折本币',
            children: q ? <Money value={q.amount_base_total} /> : <Empty />,
          },
          {
            key: 'cost_total',
            label: '成本合计',
            children: q ? <Money value={q.cost_total} /> : <Empty />,
          },
          {
            key: 'gross_profit',
            label: '毛利',
            children: q ? (
              <span style={{ color: q.gross_profit < 0 ? '#cf1322' : undefined }}>
                <Money value={q.gross_profit} />
              </span>
            ) : (
              <Empty />
            ),
          },
          {
            key: 'margin_rate',
            label: '毛利率',
            children: q ? (
              <span style={{ color: q.margin_rate < 0 ? '#cf1322' : undefined }}>
                {formatPercent(q.margin_rate)}
              </span>
            ) : (
              <Empty />
            ),
          },
          { key: 'remark', label: '备注', span: 2, children: q?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(q?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(q?.updated_at) },
        ]}
      />

      <Card title="明细" size="small" style={{ marginTop: 16 }}>
        <Table
          rowKey="id"
          size="small"
          dataSource={q?.items ?? []}
          pagination={false}
          scroll={{ x: 1100 }}
          locale={{ emptyText: '没有明细' }}
        >
          <Table.Column
            dataIndex="sku_code"
            title="SKU"
            width={140}
            render={(value: string) => <Mono>{value}</Mono>}
          />
          <Table.Column
            dataIndex="name_cn"
            title="品名 / 规格"
            render={(value: string, record: QuotationItem) => (
              <div className="cell-stack">
                <span className="cell-strong">{value || <Empty />}</span>
                {record.spec && <span className="cell-sub">{record.spec}</span>}
              </div>
            )}
          />
          <Table.Column dataIndex="unit" title="单位" width={70} render={(v: string) => v || <Empty />} />
          <Table.Column
            dataIndex="qty"
            title="数量"
            width={90}
            align="right"
            render={(value: number) => value.toLocaleString('zh-CN')}
          />
          <Table.Column
            dataIndex="unit_price"
            title="单价"
            width={110}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="amount"
            title="金额"
            width={120}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="amount_base"
            title="折本币"
            width={120}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="cost_price"
            title="单位成本"
            width={110}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="cost_amount"
            title="成本额"
            width={120}
            align="right"
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            title="毛利"
            width={110}
            align="right"
            render={(_, record: QuotationItem) => {
              const gross = record.amount_base - record.cost_amount
              return (
                <span style={{ color: gross < 0 ? '#cf1322' : undefined }}>
                  <Money value={gross} />
                </span>
              )
            }}
          />
        </Table>
      </Card>

      <Modal
        open={versionOpen}
        title="另存新版本"
        okText="保存为新版本"
        cancelText="取消"
        confirmLoading={acting}
        onOk={saveVersion}
        onCancel={() => setVersionOpen(false)}
      >
        <p>
          单号 <span className="cell-mono">{q?.doc_no}</span> 不变，版本号加一，
          新版从草稿开始，明细与成本快照照抄上一版。
        </p>
        <Input
          placeholder={q?.expired ? '必须填写新的有效期，如 2026-12-31' : '有效期留空则沿用上一版'}
          value={newValidUntil}
          onChange={(e) => setNewValidUntil(e.target.value)}
          addonBefore="有效期"
        />
      </Modal>
    </Show>
  )
}
