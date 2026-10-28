import { useShow } from '@refinedev/core'
import { Show } from '@refinedev/antd'
import { Descriptions, Table, Tag, Typography } from 'antd'

import { Empty, Mono, Money } from '@/components/fields'
import {
  paymentDirectionColor,
  paymentDirectionLabel,
  paymentTargetLabel,
  settlementMethodLabel,
} from '@/constants/catalog'
import type { Payment, PaymentAllocation } from '@/types'
import { formatDate, formatTime } from '@/utils/format'

/**
 * 收付款详情。
 *
 * 汇率那一行标「到账日」：它取自到账当天的汇率表，与应收单上锁的开单汇率
 * 不是同一个数。两者的差就是这笔钱的汇兑损益，所以两个汇率都值得摆出来——
 * 这里只列自己的，应收侧的汇率在应收详情页。
 */
export function PaymentShow() {
  const { result, query } = useShow<Payment>({ resource: 'payments' })

  const allocations = result?.allocations ?? []
  const allocated = allocations.reduce((sum, a) => sum + a.amount, 0)
  const outstanding = (result?.amount ?? 0) - allocated

  return (
    <Show
      isLoading={query.isLoading}
      title={result?.doc_no ?? '收付款详情'}
    >
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
            key: 'direction',
            label: '方向',
            children: (
              <Tag color={paymentDirectionColor(result?.direction)}>
                {paymentDirectionLabel(result?.direction)}
              </Tag>
            ),
          },
          {
            key: 'partner',
            label: result?.direction === 20 ? '供应商' : '客户',
            children: result?.partner?.name || <Empty />,
          },
          {
            key: 'pay_date',
            label: '到账日',
            children: formatDate(result?.pay_date),
          },
          { key: 'currency', label: '币种', children: result?.currency_code || <Empty /> },
          {
            key: 'exchange_rate',
            label: '汇率（到账日）',
            children: result
              ? result.exchange_rate === 1
                ? '1'
                : result.exchange_rate.toFixed(4)
              : <Empty />,
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
            key: 'allocated',
            label: '已核销',
            children:
              allocated > 0 ? (
                <Money value={allocated} />
              ) : (
                <Tag>{result?.direction === 20 ? '预付' : '预收'}</Tag>
              ),
          },
          {
            key: 'outstanding',
            label: '未核销',
            children: outstanding > 0.0001 ? <Money value={outstanding} /> : <Empty />,
          },
          {
            key: 'method',
            label: '结算方式',
            children: settlementMethodLabel(result?.method),
          },
          {
            key: 'reference',
            label: '水单号',
            children: result?.reference ? <Mono>{result.reference}</Mono> : <Empty />,
          },
          { key: 'remark', label: '备注', span: 3, children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      <Typography.Title level={5} style={{ marginTop: 24 }}>
        核销明细
      </Typography.Title>

      <Table<PaymentAllocation>
        dataSource={allocations}
        rowKey="id"
        size="small"
        pagination={false}
        locale={{ emptyText: '这笔钱还没有核销到任何单据（预收 / 预付）' }}
        columns={[
          {
            dataIndex: 'target_doc_no',
            title: '目标单据',
            render: (value: string) => <Mono>{value}</Mono>,
          },
          {
            dataIndex: 'target_type',
            title: '类型',
            width: 110,
            render: (value: number) => <Tag>{paymentTargetLabel(value)}</Tag>,
          },
          {
            dataIndex: 'amount',
            title: '核销金额（原币）',
            width: 180,
            align: 'right',
            render: (value: number) => <Money value={value} />,
          },
          {
            dataIndex: 'amount_base',
            title: '核销金额（本币）',
            width: 180,
            align: 'right',
            render: (value: number) => <Money value={value} />,
          },
        ]}
      />
    </Show>
  )
}
