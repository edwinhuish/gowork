import type { KeyboardEvent, MouseEvent, ReactNode } from 'react'
import { DeleteButton } from '@refinedev/antd'
import { Button, Pagination, Popconfirm, Skeleton, Tag, Typography } from 'antd'
import type { PaginationProps, TablePaginationConfig, TableProps } from 'antd'

import { EntityTypeTag, Mono, RoleTag } from './fields'
import { PartnerAvatar } from './partner-avatar'
import type { Partner } from '@/types'

/**
 * 往来单位（客户 / 供应商 / 货代）的卡片视图。
 *
 * 三个角色表共用这一个组件：它们是同一张 md_partner 的三种角色切片，
 * 卡片该显示哪些信息如果各写一份，改一处必然漏两处。
 * 差异收敛成一个开关 `showCustomerFacts`：货代没有商务条件与分组，
 * 这与货代表单不给那几项是同一个结论。
 *
 * 分页要自己渲染：`<Table>` 自带分页条，换成 grid 之后那条不见了，
 * 但当前页 / 页大小 / 总数仍由 useTable 持有 —— 这里只是换了个出口。
 */

/** 卡片按行数排布，12 的倍数能让常见宽度下最后一行排满，不留半排孤儿卡。 */
export const PARTNER_CARD_PAGE_SIZES = ['12', '24', '48']

export interface PartnerCardsProps {
  rows: Partner[]
  /** 直接透传 useTable 的 loading：它可能是 boolean 也可能是一组 SpinProps。 */
  loading?: TableProps<Partner>['loading']
  /** 由 useTable 的分页状态转出来的 props，见 `toCardPager`。 */
  pager?: PaginationProps
  /** 卡片右下角的动作；货代列表没有操作列，不传即可。 */
  renderActions?: (record: Partner) => ReactNode
  /** 展示客户侧的分组 / 区域 / 商务条件。 */
  showCustomerFacts?: boolean
  /** 点卡片进详情。 */
  onShow: (record: Partner) => void
  /** 空态：没有数据时显示（一般是「重置筛选 + 新建」那一组）。 */
  empty?: ReactNode
}

/**
 * 把 useTable 的分页状态转成 antd Pagination 的 props。
 *
 * 抽出来而不是让每个列表各写一遍：要复制六个字段，
 * 漏掉 `pageSizeOptions` 就会变成列表视图能改页大小、卡片视图不能。
 */
export function toCardPager(
  pager: TablePaginationConfig | false | undefined,
  showTotal: (total: number) => string,
  pageSizeOptions: string[] = PARTNER_CARD_PAGE_SIZES,
): PaginationProps | undefined {
  if (!pager || typeof pager !== 'object') return undefined
  return {
    current: pager.current ?? 1,
    pageSize: pager.pageSize ?? Number(pageSizeOptions[0]),
    total: pager.total ?? 0,
    showSizeChanger: true,
    pageSizeOptions,
    showTotal,
    onChange: pager.onChange as (page: number, pageSize: number) => void,
  }
}

export function PartnerCards({
  rows,
  loading,
  pager,
  renderActions,
  showCustomerFacts = false,
  onShow,
  empty,
}: PartnerCardsProps) {
  // 骨架屏的结构与真实卡片一致（头像 + 标题 + 三行信息），
  // 数据到了不会整页跳一下。
  if (loading && rows.length === 0) {
    return (
      <div className="partner-grid">
        {Array.from({ length: 8 }).map((_, i) => (
          <div className="partner-card partner-card--ghost" key={i}>
            <div className="partner-card-head">
              <Skeleton.Avatar active size={40} />
              <Skeleton active title={{ width: '60%' }} paragraph={false} />
            </div>
            <Skeleton active paragraph={{ rows: 3 }} title={false} />
          </div>
        ))}
      </div>
    )
  }

  if (rows.length === 0) return <>{empty ?? null}</>

  return (
    <>
      <div className="partner-grid">
        {rows.map((record) => (
          <article
            key={record.id}
            className="partner-card"
            role="button"
            tabIndex={0}
            aria-label={`${record.code} ${record.name}`}
            onClick={() => onShow(record)}
            onKeyDown={(event: KeyboardEvent<HTMLElement>) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                onShow(record)
              }
            }}
          >
            <div className="partner-card-head">
              {/* 头像与列表视图调用同一个组件、同一个默认尺寸：
                  换视图时它必须长成同一个样子，否则用户会以为是两条不同的数据。 */}
              <PartnerAvatar name={record.name} logoUrl={record.logo_url} />
              <div className="partner-card-heading">
                <Typography.Text className="partner-card-name" title={record.name} strong>
                  {record.name}
                </Typography.Text>
                {/* 软删标记顶的就是原来"停用"那个位置：它是这条记录唯一的状态，
                    已删除的行在列表里只能做恢复这一件事。 */}
                <span className="partner-card-code">
                  <Mono>{record.code}</Mono>
                  {record.deleted_at ? <Tag bordered={false}>已删除</Tag> : null}
                </span>
              </div>
            </div>

            <div className="partner-card-tags">
              <RoleTag value={record.role} />
              <EntityTypeTag value={record.entity_type} />
              {record.grade ? <Tag bordered={false}>{record.grade} 级</Tag> : null}
            </div>

            <div className="partner-card-facts">
              <Fact label="联系人" value={record.contact} />
              <Fact label="电话" value={record.phone} />
              <Fact label="邮箱" value={record.email} />
              {showCustomerFacts ? (
                <>
                  <Fact label="国家/地区" value={record.country} />
                  <Fact label="分组" value={record.customer_group} />
                  <Fact label="区域" value={record.region} />
                  <Fact label="结算" value={record.settlement_method} />
                  {/* 0 天是"款到发货"而不是"当天该付"，与列表那列同口径。 */}
                  <Fact
                    label="账期"
                    value={record.credit_days ? `${record.credit_days} 天` : '款到发货'}
                  />
                </>
              ) : null}
            </div>

            <div className="partner-card-foot">
              <span className="partner-card-owner" title={record.owner_name ?? ''}>
                负责人 {record.owner_name || '未分配'}
              </span>
              {/* 整张卡是"进详情"的点击区，操作按钮必须先把事件摁住。 */}
              <span
                className="partner-card-actions"
                onClick={(event: MouseEvent<HTMLSpanElement>) => event.stopPropagation()}
                onKeyDown={(event: KeyboardEvent<HTMLSpanElement>) => event.stopPropagation()}
              >
                {renderActions?.(record)}
              </span>
            </div>
          </article>
        ))}
      </div>
      {pager ? (
        <div className="partner-grid-pager">
          <Pagination {...pager} />
        </div>
      ) : null}
    </>
  )
}

/**
 * 一行「标签：值」。
 *
 * 标签与值成对绑定而不是自由排版：卡片列宽是浮动的，
 * 两者不绑在一起，窄列时会错位到看不出谁是谁的值。
 */
function Fact({ label, value }: { label: string; value?: string }) {
  return (
    <div className="partner-card-fact">
      <span className="partner-card-fact-label">{label}</span>
      <span className="partner-card-fact-value" title={value || ''}>
        {value || '—'}
      </span>
    </div>
  )
}

interface PartnerRowActionsProps {
  record: Partner
  /** 要删的资源：customers / suppliers / forwarders，决定调哪个接口。 */
  resource: string
  /** 主体称谓，用于确认文案：客户 / 供应商 / 货代。 */
  entityLabel: string
  onRestore: (record: Partner) => void
}

/**
 * 表格行与卡片共用的「删除 / 恢复」。
 *
 * 已软删的主体只能恢复：对它们调编辑与删除都会 404，按钮摆出来也没用。
 */
export function PartnerRowActions({
  record,
  resource,
  entityLabel,
  onRestore,
}: PartnerRowActionsProps) {
  if (record.deleted_at) {
    return (
      <Popconfirm
        title={`恢复这个${entityLabel}？`}
        description="只去掉删除标记，删除期间改过的引用不会自动指回来。"
        okText="恢复"
        cancelText="返回"
        onConfirm={() => onRestore(record)}
      >
        <Button size="small" type="link">
          恢复
        </Button>
      </Popconfirm>
    )
  }
  return (
    <DeleteButton
      size="small"
      resource={resource}
      recordItemId={record.id}
      confirmTitle={`删除这个${entityLabel}？`}
      confirmOkText="删除"
      confirmCancelText="返回"
    />
  )
}
