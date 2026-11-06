import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions, Typography } from 'antd'
import { Link } from 'react-router'

import { AttachmentPanel } from '@/components/attachment-panel'
import { DocChain } from '@/components/doc-chain'
import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import {
  SETTLEMENT_STATUS_OPTIONS,
  settlementMethodLabel,
  settlementSourceLabel,
} from '@/constants/catalog'
import type { Settlement, SettlementAllocation } from '@/types'
import { formatDate, formatTime } from '@/utils/format'

import type { SettlementDirection } from './form-fields'

/**
 * 来源单据的详情页路径。
 *
 * source_type 的取值见后端 model.SettlementSource：10 销售订单 / 20 出货批次 /
 * 30 采购订单 / 40 合同分期。这里只是把类型翻译成路由，不含业务判断。
 */
function sourceHref(type: number, id: number): string | undefined {
  switch (type) {
    case 10:
      return `/sales-orders/show/${id}`
    case 20:
      return `/shipments/show/${id}`
    case 30:
      return `/purchase-orders/show/${id}`
    case 40:
      return `/contracts/show/${id}`
    default:
      return undefined
  }
}

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

  // 金额是定点字符串，这里的差额只用于显示；服务端按末位精确算。
  const outstanding = result ? Number(result.amount) - Number(result.settled_amount) : 0

  const href = result ? sourceHref(result.source_type, result.source_id) : undefined
  const allocations = result?.allocations ?? []
  // 收款这一环要接上真实核销记录：只按单据推导的话，一笔已经收齐的应收
  // 在链上仍然画成"还没有收款"，看的人会以为钱没到。
  // 多笔时不给链接（链上一格放不下多张单），单号写成「N 笔」。
  const paymentChain =
    allocations.length === 0
      ? {}
      : allocations.length === 1
        ? {
            payment: {
              docNo: allocations[0].payment_doc_no,
              href: `/payments/show/${allocations[0].payment_id}`,
            },
          }
        : { payment: { docNo: `${allocations.length} 笔` } }
  // 应付那侧的标签是采购词（报价/合同/出货），拿销售的单据链去套会自相矛盾，
  // 所以只在应收上画链；应付的来源链接放在 Descriptions 那一行里。
  const chainDocs = {
    receivable: { docNo: result?.doc_no },
    ...(result?.source_type === 10 ? { order: { docNo: result.source_doc_no, href } } : {}),
    ...(result?.source_type === 20 ? { shipment: { docNo: result.source_doc_no, href } } : {}),
    ...(result?.source_type === 40 ? { contract: { docNo: result.source_doc_no, href } } : {}),
    ...paymentChain,
  }

  return (
    <Show isLoading={query.isLoading} title={result?.doc_no ?? (isReceivable ? '应收详情' : '应付详情')}>
      {isReceivable ? <DocChain current="receivable" docs={chainDocs} /> : null}

      <Descriptions
        column={{ xs: 1, sm: 2, md: 2, lg: 2, xl: 3 }}
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
              <OptionTag value={result?.status} options={SETTLEMENT_STATUS_OPTIONS} />
            ),
          },
          {
            key: 'source',
            label: '来源单据',
            children: result ? (
              href ? (
                <Link to={href}>
                  {settlementSourceLabel(result.source_type)}{' '}
                  <Mono>{result.source_doc_no}</Mono>
                </Link>
              ) : (
                `${settlementSourceLabel(result.source_type) ?? ''} ${result.source_doc_no}`.trim()
              )
            ) : (
              <Empty />
            ),
          },
          { key: 'currency', label: '币种', children: result?.currency_code || <Empty /> },
          {
            key: 'exchange_rate',
            label: '汇率（开单时锁定）',
            children: result ? (Number(result.exchange_rate) === 1 ? '1' : Number(result.exchange_rate).toFixed(4)) : <Empty />,
          },
          {
            key: 'amount',
            label: '金额（原币）',
            children: result ? <Money value={result.amount} currency={result.currency_code} /> : <Empty />,
          },
          {
            key: 'amount_base',
            label: '金额（本币）',
            children: result ? <Money value={result.amount_base} /> : <Empty />,
          },
          {
            key: 'settled',
            label: '已结清',
            children: result && Number(result.settled_amount) > 0 ? (
              <Money value={result.settled_amount} currency={result.currency_code} />
            ) : (
              <Empty />
            ),
          },
          {
            key: 'outstanding',
            label: '未结清',
            children: result?.status === 90 ? <Empty /> : <Money value={outstanding} currency={result?.currency_code} />,
          },
          {
            key: 'due_date',
            label: '到期日',
            children: result ? formatDate(result.due_date) : <Empty />,
          },
          // 'filled'：占满这一行剩下的列。column 是响应式的（窄屏只有 1 列），
          // 写死 2 会超出列数 —— antd 告警，这一格还会被裁回 1 列。
          { key: 'remark', label: '备注', span: 'filled', children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      {/* 已结清金额是核销行加总出来的派生值。只给一个总数时，
          "这 12,000 是谁付的、哪天到的、按哪个汇率折的"都答不上来，
          对账的人只能回到收付款列表里逐张翻。 */}
      <Typography.Title level={5} style={{ marginTop: 24 }}>
        {isReceivable ? '收款记录' : '付款记录'}
      </Typography.Title>

      <ResponsiveTable<SettlementAllocation>
        rows={allocations}
        rowKey="id"
        size="small"
        pagination={false}
        empty={
          isReceivable
            ? '这笔应收还没收到过钱；预收挂在收付款单上，核销之后才会出现在这里'
            : '这笔应付还没付过钱；预付挂在收付款单上，核销之后才会出现在这里'
        }
        columns={[
          {
            dataIndex: 'pay_date',
            title: '到账日',
            width: 120,
            render: (value: string) => formatDate(value),
          },
          {
            dataIndex: 'payment_doc_no',
            title: isReceivable ? '收款单' : '付款单',
            // 这一行回答的就是"钱从哪来"，单号必须能点进去看水单与其它核销行。
            render: (value: string, record: SettlementAllocation) => (
              <Link to={`/payments/show/${record.payment_id}`}>
                <Mono>{value}</Mono>
              </Link>
            ),
          },
          {
            dataIndex: 'method',
            title: '结算方式',
            width: 110,
            render: (value: string) => settlementMethodLabel(value) || <Empty />,
          },
          {
            dataIndex: 'reference',
            title: '水单号',
            width: 150,
            render: (value: string) => (value ? <Mono>{value}</Mono> : <Empty />),
          },
          {
            dataIndex: 'amount',
            title: '核销金额（原币）',
            width: 180,
            align: 'right',
            render: (value: string, record: SettlementAllocation) => (
              <Money value={value} currency={record.currency_code} />
            ),
          },
          {
            dataIndex: 'amount_base',
            title: '核销金额（本币）',
            width: 170,
            align: 'right',
            // 与收付款详情同一条口径：本币不跟原币符号。
            render: (value: string) => <Money value={value} />,
          },
          {
            dataIndex: 'exchange_rate',
            title: '汇率（到账日）',
            width: 130,
            align: 'right',
            // 到账日汇率与单头上锁的那个通常不同，差额就是这笔的汇兑损益；
            // 两个数并排才看得出差在哪，只显示一个时对不上账。
            render: (value: string) => (
              <span className="cell-mono">{Number(value) === 1 ? '1' : Number(value).toFixed(4)}</span>
            ),
          },
        ]}
      />

      {/* 对账单与发票是"这笔账凭什么这么算"的证据，对账时双方各执一份。
          应收挂客户回签的对账单与开给客户的发票，应付挂供应商开来的发票——
          两侧的单据根本不是同一批文件，所以标题随方向走。 */}
      {result?.id ? (
        <AttachmentPanel
          entityType={isReceivable ? 'receivable' : 'payable'}
          entityId={result.id}
          title={isReceivable ? '对账单与发票' : '供应商发票与对账单'}
        />
      ) : null}
    </Show>
  )
}
