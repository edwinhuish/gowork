import { RightOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router'

/**
 * 单据链。
 *
 * 一笔外销生意的顺序是固定的：报价 → 合同 → 销售订单 → 出货 → 应收 → 收款。
 * 详情页单独看一张单，最容易丢的恰恰是"我在整条链的哪一环、上下游是谁"，
 * 于是所有的查找都退化成回去搜单号。
 *
 * 每一环有三种状态，颜色与形状都不一样：
 *   - 已关联且能跳转：实线可点（is-done + is-clickable）
 *   - 已关联但没页面可跳（如只知道单号）：实线不可点
 *   - 还没这一环：虚线灰字（is-pending）
 * 没关联不代表出错（报价本来就可以不签合同直接下订单），所以不给红色。
 */
export type DocChainKey = 'quotation' | 'contract' | 'order' | 'shipment' | 'receivable' | 'payment'

const STAGES: { key: DocChainKey; label: string }[] = [
  { key: 'quotation', label: '报价单' },
  { key: 'contract', label: '合同' },
  { key: 'order', label: '销售订单' },
  { key: 'shipment', label: '出货' },
  { key: 'receivable', label: '应收' },
  { key: 'payment', label: '收款' },
]

export type DocChainEntry = {
  /** 单号，用来在节点上直接认单；只有 href 时退化成环名。 */
  docNo?: string | null
  /** 站内路径。没有就表示这一环只做展示，点不进去。 */
  href?: string | null
}

export type DocChainDocs = Partial<Record<DocChainKey, DocChainEntry>>

export function DocChain({
  current,
  docs,
  hide = [],
}: {
  current: DocChainKey
  docs: DocChainDocs
  /** 这条业务链上不存在的环节（如采购单没有报价/合同），隐藏比留个空环清楚。 */
  hide?: DocChainKey[]
}) {
  const navigate = useNavigate()
  const stages = STAGES.filter((stage) => !hide.includes(stage.key))

  return (
    <div className="doc-chain">
      {stages.map((stage, index) => {
        const entry = docs[stage.key]
        const isCurrent = stage.key === current
        const clickable = !!entry?.href && !isCurrent
        const classes = ['doc-chain-node']
        if (isCurrent) classes.push('is-current')
        else if (entry) classes.push('is-done')
        else classes.push('is-pending')
        if (clickable) classes.push('is-clickable')

        const text = entry?.docNo || stage.label
        return (
          <span key={stage.key} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            {index > 0 && <RightOutlined className="doc-chain-arrow" />}
            <span
              className={classes.join(' ')}
              title={entry ? text : `${stage.label}：这一环还没有单据`}
              role={clickable ? 'button' : undefined}
              tabIndex={clickable ? 0 : undefined}
              onClick={clickable ? () => navigate(entry!.href!) : undefined}
              onKeyDown={
                clickable
                  ? (e) => {
                      if (e.key === 'Enter' || e.key === ' ') navigate(entry!.href!)
                    }
                  : undefined
              }
            >
              {stage.label}
              {entry?.docNo ? <span className="cell-mono">{entry.docNo}</span> : null}
            </span>
          </span>
        )
      })}
    </div>
  )
}
