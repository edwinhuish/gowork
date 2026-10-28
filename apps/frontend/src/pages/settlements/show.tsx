import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions, Tag } from 'antd'

import { Empty, Money } from '@/components/fields'
import {
  settlementSourceLabel,
  settlementStatusColor,
  settlementStatusLabel,
} from '@/constants/catalog'
import type { Settlement } from '@/types'
import { formatDate, formatTime } from '@/utils/format'

import type { SettlementDirection } from './form-fields'

/**
 * 应收 / 应付详情。两个模块共用。
 *
 * 把「原币 / 汇率 / 本币」三行都列出来：汇率是开单时锁的，
 * 只显示本币金额的话，过几个月没人能复算出这个数是按哪个汇率折的。
 */
export function SettlementShow({
  resource,
  direction,
}: {
  resource: 'receivables' | 'payables'
  direction: SettlementDirection
}) {
  const isReceivable = direction === 'receivable'
  const { result, query } = useShow<Settlement>({ resource })

  const outstanding = result ? result.amount - result.settled_amount : 0

  return (
    <Show isLoading={query.isLoading} title={result?.doc_no ?? (isReceivable ? '应收详情' : '应付详情')}>
      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="middle"
        items={[
          {
            key: 'doc_no',
            label: '单号',
            children: <span className="cell-mono">{result?.doc_no}</span>,
          },
          {
            key: 'partner',
            label: isReceivable ? '客户' : '供应商',
            children: result?.partner?.name || <Empty />,
          },
          {
            key: 'status',
            label: '状态',
            children: (
              <Tag color={settlementStatusColor(result?.status)}>
                {settlementStatusLabel(result?.status)}
              </Tag>
            ),
          },
          {
            key: 'source',
            label: '来源单据',
            children: result
              ? `${settlementSourceLabel(result.source_type) ?? ''} ${result.source_doc_no}`.trim()
              : <Empty />,
          },
          { key: 'currency', label: '币种', children: result?.currency_code || <Empty /> },
          {
            key: 'exchange_rate',
            label: '汇率（开单时锁定）',
            children: result ? (result.exchange_rate === 1 ? '1' : result.exchange_rate.toFixed(4)) : <Empty />,
          },
          {
            key: 'amount',
            label: '金额（原币）',
            children: result ? <Money value={result.amount} /> : <Empty />,
          },
          {
            key: 'amount_base',
            label: '金额（本币）',
            children: result ? <Money value={result.amount_base} /> : <Empty />,
          },
          {
            key: 'settled',
            label: '已结清',
            children: result && result.settled_amount > 0 ? (
              <Money value={result.settled_amount} />
            ) : (
              <Empty />
            ),
          },
          {
            key: 'outstanding',
            label: '未结清',
            children: result?.status === 90 ? <Empty /> : <Money value={outstanding} />,
          },
          {
            key: 'due_date',
            label: '到期日',
            children: result ? formatDate(result.due_date) : <Empty />,
          },
          { key: 'remark', label: '备注', span: 2, children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />
    </Show>
  )
}
