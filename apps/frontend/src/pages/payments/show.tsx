import { useShow } from '@refinedev/core'
import { Show } from '@refinedev/antd'
import { Button, Descriptions, Popconfirm, Tag, Typography, message } from 'antd'

import { paymentApi } from '@/api/payments'
import { AttachmentPanel } from '@/components/attachment-panel'
import { DocChain } from '@/components/doc-chain'
import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import {
  PAYMENT_DIRECTION_OPTIONS,
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

  // 金额是定点字符串：下面的合计只用于显示，服务端按末位精确算。
  const allocations = result?.allocations ?? []

  // 撤销核销后重取详情：这笔钱的核销行与目标单据的已核销额都变了。
  const removeAllocation = async (allocationId: number) => {
    if (!result) return
    try {
      await paymentApi.removeAllocation(result.id, allocationId)
      message.success('已撤销这条核销')
      await query.refetch()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '撤销失败')
    }
  }
  const allocated = allocations.reduce((sum, a) => sum + Number(a.amount), 0)
  const outstanding = Number(result?.amount ?? 0) - allocated

  return (
    <Show
      isLoading={query.isLoading}
      title={result?.doc_no ?? '收付款详情'}
    >
      {/* 只有收款才画销售的单据链：付款那侧的上下游是采购单与应付单，
          用「报价 / 合同 / 出货」去描述会自相矛盾。 */}
      {result?.direction === 10 ? (
        <DocChain
          current="payment"
          docs={{
            receivable: allocations.length
              ? {
                  docNo:
                    allocations.length > 1
                      ? `${allocations[0].target_doc_no} 等 ${allocations.length} 张`
                      : allocations[0].target_doc_no,
                  href: `/receivables/show/${allocations[0].target_id}`,
                }
              : undefined,
            payment: { docNo: result.doc_no },
          }}
        />
      ) : null}

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
              <OptionTag value={result?.direction} options={PAYMENT_DIRECTION_OPTIONS} />
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
              ? Number(result.exchange_rate) === 1
                ? '1'
                : Number(result.exchange_rate).toFixed(4)
              : <Empty />,
          },
          {
            key: 'amount',
            label: '金额（原币）',
            children: result ? <Money value={result.amount} currency={result?.currency_code} /> : <Empty />,
          },
          {
            key: 'amount_base',
            label: '金额（本币）',
            children: result ? <Money value={result.amount_base} currency={result?.currency_code} /> : <Empty />,
          },
          {
            key: 'allocated',
            label: '已核销',
            children:
              allocated > 0 ? (
                <Money value={allocated} currency={result?.currency_code} />
              ) : (
                <Tag>{result?.direction === 20 ? '预付' : '预收'}</Tag>
              ),
          },
          {
            key: 'outstanding',
            label: '未核销',
            children: outstanding > 0.0001 ? <Money value={outstanding} currency={result?.currency_code} /> : <Empty />,
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
          // 'filled'：占满这一行剩下的列。column 是响应式的（窄屏只有 1 列），
          // 写死 2 / 3 会超出列数 —— antd 告警，这一格还会被裁回 1 列。
          { key: 'remark', label: '备注', span: 'filled', children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      <Typography.Title level={5} style={{ marginTop: 24 }}>
        核销明细
      </Typography.Title>

      <ResponsiveTable<PaymentAllocation>
        rows={allocations}
        rowKey="id"
        size="small"
        pagination={false}
        empty="这笔钱还没有核销到任何单据（预收 / 预付）"
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
            render: (value: string) => <Money value={value} currency={result?.currency_code} />,
          },
          {
            dataIndex: 'amount_base',
            title: '核销金额（本币）',
            width: 180,
            align: 'right',
            render: (value: string) => <Money value={value} currency={result?.currency_code} />,
          },
          {
            // 撤销一条核销（T-52）：不删整张单，只回滚这一行。
            title: '操作',
            width: 90,
            render: (_: unknown, record: PaymentAllocation) => (
              <Popconfirm
                title="撤销这条核销？"
                description="只回滚这一行：目标单据的已核销额减回去，收付款单本身不变。"
                okText="撤销"
                cancelText="返回"
                onConfirm={() => void removeAllocation(record.id)}
              >
                <Button size="small" type="link" danger>
                  撤销
                </Button>
              </Popconfirm>
            ),
          },
        ]}
      />

      {/* 银行回单与水单是收付款的凭据：没有它，核销只能靠人记得"这笔钱是谁打的"，
          对账时双方各执一份、谁也说服不了谁。 */}
      {result?.id ? (
        <AttachmentPanel entityType="payment" entityId={result.id} title="银行回单" />
      ) : null}
    </Show>
  )
}
