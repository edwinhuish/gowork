import { useState } from 'react'
import { useInvalidate, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import { Button, Card, Descriptions, message, Popconfirm, Space, Table, Tag, Typography } from 'antd'

import { stockDocApi } from '@/api/stock-documents'
import { AttachmentPanel } from '@/components/attachment-panel'
import { Empty, Mono, OptionTag } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import {
  ORDER_STATUS_OPTIONS,
  STOCK_DOC_COUNT,
  STOCK_DOC_TYPE_OPTIONS,
} from '@/constants/catalog'
import type { StockDocument, StockDocumentItem } from '@/types'
import { formatDate, formatQty, formatTime } from '@/utils/format'

export function StockDocumentShow() {
  const { result, query } = useShow<StockDocument>({ resource: 'stock-documents' })
  const invalidate = useInvalidate()
  const [acting, setActing] = useState(false)

  const d = result
  const isCount = d?.type === STOCK_DOC_COUNT
  const draft = d?.status === 10

  const confirm = async () => {
    if (!d) return
    setActing(true)
    try {
      await stockDocApi.confirm(d.id)
      message.success(
        isCount ? '盘点已确认，差异已生成盈亏流水' : '单据已确认，库存已更新',
      )
      await invalidate({ resource: 'stock-documents', invalidates: ['detail', 'list'] })
      // 库存与流水都变了，顺手让它们的缓存也失效。
      await invalidate({ resource: 'stock-snapshots', invalidates: ['list'] })
      await invalidate({ resource: 'stock-movements', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '确认失败')
    } finally {
      setActing(false)
    }
  }

  // 红冲（T-52c）：生成反向单据，库存回到本次确认之前。
  const reverse = async () => {
    if (!d) return
    setActing(true)
    try {
      await stockDocApi.reverse(d.id)
      message.success('已生成红冲单，库存已恢复')
      await invalidate({ resource: 'stock-documents', invalidates: ['detail', 'list'] })
      await invalidate({ resource: 'stock-snapshots', invalidates: ['list'] })
      await invalidate({ resource: 'stock-movements', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '红冲失败')
    } finally {
      setActing(false)
    }
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={d ? `出入库单 ${d.doc_no}` : '出入库单详情'}
      headerButtons={
        <Space wrap>
          {draft && <EditButton resource="stock-documents" recordItemId={d?.id} />}
          {draft && (
            <Popconfirm
              title={isCount ? '确认这张盘点单？' : '确认这张单据？'}
              description={
                isCount
                  ? '确认后按「实盘 − 当时账面」生成盈亏流水，不能撤销。'
                  : '确认后库存立即变更，单据不可再改；要纠正只能做反向单。'
              }
              okText="确认"
              cancelText="返回"
              onConfirm={confirm}
            >
              <Button type="primary" loading={acting}>
                确认
              </Button>
            </Popconfirm>
          )}
          {/* 红冲只给"已确认、没被红冲过、自己也不是红冲单"的单据：
              盘点单按差异生成流水，再冲一次等于抵消差异而不是改回库存。 */}
          {d?.status === 20 && !d.reversed && !d.reversal_of_id && !isCount && (
            <Popconfirm
              title="红冲这张单据？"
              description="会生成一张方向相反的单据并立即确认，库存回到本次确认之前；原单保留可查。"
              okText="红冲"
              cancelText="返回"
              onConfirm={reverse}
            >
              <Button danger loading={acting}>
                红冲
              </Button>
            </Popconfirm>
          )}
        </Space>
      }
    >
      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="small"
        items={[
          {
            key: 'doc_no',
            label: '单号',
            children: <span className="cell-mono">{d?.doc_no}</span>,
          },
          {
            key: 'type',
            label: '类型',
            // 出入库类型是身份（方向不同），不是状态，所以字典里没有 tone。
            children: d ? <OptionTag value={d.type} options={STOCK_DOC_TYPE_OPTIONS} /> : <Empty />,
          },
          {
            key: 'status',
            label: '状态',
            children: d ? (
              <OptionTag value={d.status} options={ORDER_STATUS_OPTIONS} />
            ) : (
              <Empty />
            ),
          },
          {
            key: 'doc_date',
            label: '单据日期',
            children: d ? formatDate(d.doc_date) : <Empty />,
          },
          {
            key: 'partner',
            label: '往来单位',
            children: d?.partner?.name || <Empty />,
          },
          {
            key: 'source_doc_no',
            label: '来源单号',
            children: d?.source_doc_no ? <Mono>{d.source_doc_no}</Mono> : <Empty />,
          },
          {
            key: 'posted_at',
            label: '确认时间',
            children: d?.posted_at ? formatTime(d.posted_at) : <Empty />,
          },
          // 两个标识（T-52c）：红冲单要能看出冲的是谁，原单要能看出已被冲过。
          ...(d?.reversal_of_id
            ? [
                {
                  key: 'reversal',
                  label: '红冲自',
                  children: <Mono>{d.source_doc_no}</Mono>,
                },
              ]
            : []),
          ...(d?.reversed
            ? [
                {
                  key: 'reversed',
                  label: '红冲状态',
                  children: <Tag color="error">已被红冲</Tag>,
                },
              ]
            : []),
          { key: 'remark', label: '备注', span: 2, children: d?.remark || <Empty /> },
        ]}
      />

      <Card title="明细" size="small" style={{ marginTop: 16 }}>
        <ResponsiveTable
          rowKey="id"
          size="small"
          rows={d?.items ?? []}
          pagination={false}
          scrollX={900}
          empty="没有明细"
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
            render={(value: string, record: StockDocumentItem) => (
              <div className="cell-stack">
                <span className="cell-strong">{value || <Empty />}</span>
                {record.spec && <span className="cell-sub">{record.spec}</span>}
              </div>
            )}
          />
          <Table.Column
            dataIndex="unit"
            title="单位"
            width={70}
            render={(value: string) => value || <Empty />}
          />
          {isCount && (
            <Table.Column
              dataIndex="book_qty"
              title="账面"
              width={90}
              align="right"
              render={(value?: string | null) =>
                value === null || value === undefined ? <Empty /> : formatQty(value)
              }
            />
          )}
          <Table.Column
            dataIndex="qty"
            title={isCount ? '实盘' : '数量'}
            width={90}
            align="right"
            render={(value: string) => formatQty(value)}
          />
          {isCount && (
            <Table.Column
              title="差异"
              width={100}
              align="right"
              render={(_, record: StockDocumentItem) => {
                if (record.book_qty === null || record.book_qty === undefined) {
                  return <Empty />
                }
                // 金额与数量都是定点字符串：差异只用于显示，账实判定在确认那一刻的流水里。
                const diff = Number(record.qty) - Number(record.book_qty)
                if (Math.abs(diff) < 1e-6) {
                  return <Typography.Text type="secondary">相符</Typography.Text>
                }
                return (
                  <Typography.Text type={diff > 0 ? 'success' : 'danger'}>
                    {diff > 0 ? '+' : '−'}
                    {formatQty(Math.abs(diff))}
                  </Typography.Text>
                )
              }}
            />
          )}
          <Table.Column
            dataIndex="remark"
            title="备注"
            render={(value: string) => value || <Empty />}
          />
        </ResponsiveTable>
      </Card>
      {/* T-63：出入库单要挂签收单、过磅单、验货照片——这些是仓库与客户对账的凭据。 */}
      {result?.id ? (
        <AttachmentPanel entityType="stock_document" entityId={result.id} title="随单凭据" />
      ) : null}

    </Show>
  )
}
