import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions } from 'antd'
import { Link } from 'react-router'

import { AttachmentPanel } from '@/components/attachment-panel'
import { DocChain } from '@/components/doc-chain'
import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import {
  SETTLEMENT_STATUS_OPTIONS,
  settlementSourceLabel,
} from '@/constants/catalog'
import type { Settlement } from '@/types'
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
  // 应付那侧的标签是采购词（报价/合同/出货），拿销售的单据链去套会自相矛盾，
  // 所以只在应收上画链；应付的来源链接放在 Descriptions 那一行里。
  const chainDocs = {
    receivable: { docNo: result?.doc_no },
    ...(result?.source_type === 10 ? { order: { docNo: result.source_doc_no, href } } : {}),
    ...(result?.source_type === 20 ? { shipment: { docNo: result.source_doc_no, href } } : {}),
    ...(result?.source_type === 40 ? { contract: { docNo: result.source_doc_no, href } } : {}),
  }

  return (
    <Show isLoading={query.isLoading} title={result?.doc_no ?? (isReceivable ? '应收详情' : '应付详情')}>
      {isReceivable ? <DocChain current="receivable" docs={chainDocs} /> : null}

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
