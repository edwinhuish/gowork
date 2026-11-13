import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSelect } from '@refinedev/core'
import { Alert, Card, Flex, Input, Select, Space, Table, Tag, Typography, theme } from 'antd'
import { Link } from 'react-router'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import { FilterBar, InfiniteFooter, PageHeader, ResponsiveTable } from '@/components/page'
import { PAYMENT_TARGET_OPTIONS, paymentDirectionLabel } from '@/constants/catalog'
import { useClientFeed } from '@/hooks/use-infinite-feed'
import type { Currency, ExchangeGainLine, Partner } from '@/types'

/**
 * 损益值：正绿负红。
 *
 * 符号不是装饰——这张表要回答"这个月汇率帮我赚了还是亏了"，
 * 而"−200"和"200"在数字列里长得几乎一样。
 */
function GainValue({ value, strong }: { value?: string | null; strong?: boolean }) {
  const { token } = theme.useToken()
  // 金额是定点字符串：这里的正负判断与色条只用于显示，取值本身原样交给 Money。
  if (value === null || value === undefined) return <Empty />
  const num = Number(value)
  if (Number.isNaN(num)) return <Empty />
  const color =
    num > 0.0001 ? token.colorSuccess : num < -0.0001 ? token.colorError : undefined
  return (
    <span
      className={strong ? 'cell-strong' : undefined}
      style={{ color, fontVariantNumeric: 'tabular-nums' }}
    >
      {num > 0 ? '+' : ''}
      <Money value={value} />
    </span>
  )
}

/**
 * 汇兑损益报表（T-23）。
 *
 * 明细逐笔可核对：每一行都能追到一张收付款单和一张应收 / 应付单。
 * 与毛利分成两张表看：毛利是经营口径，损益是财务口径，混在一起就分不清
 * "赚 8%" 里有多少是报价带来的、有多少是汇率波动的。
 */
export function ExchangeGainsPage() {
  // 输入与已生效值分开：打字不触发查询，回车或失焦才生效。
  const [fromInput, setFromInput] = useState('')
  const [toInput, setToInput] = useState('')
  const [range, setRange] = useState({ from: '', to: '' })
  const [currency, setCurrency] = useState<string | undefined>()
  const [partnerId, setPartnerId] = useState<number | undefined>()

  const { data, isLoading, error } = useQuery({
    queryKey: ['report-exchange-gains', range, currency, partnerId],
    queryFn: () =>
      reportApi.exchangeGains({
        from: range.from || undefined,
        to: range.to || undefined,
        currency_code: currency,
        partner_id: partnerId,
      }),
  })

  const { options: currencyOptions } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'code',
    pagination: { mode: 'off' },
  })

  const { options: partnerOptions } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const lines = data?.lines ?? []
  const applyRange = () => setRange({ from: fromInput.trim(), to: toInput.trim() })

  // 明细是全量的，一次铺出来最多几百行 —— 分批揭示：先给一批，
  // 滚到底再放下一批。两种布局（表格 / 卡片）看到的是同一份行。
  const feed = useClientFeed(lines, 50)

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="汇兑损益"
        description="损益 = 核销原币 ×（到账汇率 − 单据锁定汇率）；应收到账汇率高记收益，应付付款汇率高记损失"
      />

      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <FilterBar
        extra={
          <Typography.Text type="secondary">
            {range.from || range.to
              ? `按到账日 ${range.from || '不限'} ~ ${range.to || '不限'}`
              : '按到账日，默认全部'}
          </Typography.Text>
        }
      >
        <Input
          value={fromInput}
          onChange={(e) => setFromInput(e.target.value)}
          onPressEnter={applyRange}
          onBlur={applyRange}
          placeholder="起始日 YYYY-MM-DD"
          style={{ width: 200 }}
          allowClear
        />
        <Input
          value={toInput}
          onChange={(e) => setToInput(e.target.value)}
          onPressEnter={applyRange}
          onBlur={applyRange}
          placeholder="截止日 YYYY-MM-DD"
          style={{ width: 200 }}
          allowClear
        />
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          style={{ width: 220 }}
          placeholder="全部往来单位"
          options={partnerOptions}
          value={partnerId}
          onChange={(v: number | undefined) => setPartnerId(v)}
        />
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          style={{ width: 180 }}
          placeholder="全部币种"
          options={currencyOptions}
          value={currency}
          onChange={(v: string | undefined) => setCurrency(v)}
        />
      </FilterBar>

      <Card variant="borderless" title="汇兑损益明细">
        <ResponsiveTable<ExchangeGainLine>
          rows={feed.rows}
          rowKey={(row) => `${row.payment_id}-${row.target_doc_no}-${row.amount}`}
          loading={isLoading}
          // 1138 = 112+140+70+148+68+160+110+100+100+130，正好落在 1440 的内容区里。
          scrollX={1138}
          empty="这段时间没有产生汇兑差额的核销"
          // 第一列是到账日，卡上该写单号：认一笔损益靠的是"哪张收付款单"。
          cardTitleKey="payment_doc_no"
          cardSummary={{
            title: '本币合计',
            facts: [
              {
                key: 'gain',
                label: '汇兑损益',
                value: <GainValue value={data?.total_gain_base} strong />,
              },
            ],
          }}
          summary={() =>
            lines.length > 0 ? (
              <Table.Summary fixed>
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={9}>
                    <span className="cell-strong">本币合计</span>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={9} align="right">
                    <GainValue value={data?.total_gain_base} strong />
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              </Table.Summary>
            ) : null
          }
        >
          <Table.Column
            dataIndex="pay_date"
            title="到账日"
            // auto 布局下列宽只是建议值，再补 nowrap：日期折成「2026-10-」+「27」
            // 会被误读成两个数，而这张表里到处是数。
            width={112}
            render={(value: string) => <span style={{ whiteSpace: 'nowrap' }}>{value}</span>}
          />
          <Table.Column
            dataIndex="payment_doc_no"
            title="收付款单"
            width={140}
            // 这一行回答「汇兑差额是哪笔钱产生的」，点进去才看得到水单与其它核销行。
            render={(value: string, row: ExchangeGainLine) => (
              <Link to={`/payments/show/${row.payment_id}`}>
                <Mono>{value}</Mono>
              </Link>
            )}
          />
          <Table.Column
            dataIndex="direction"
            title="方向"
            width={70}
            render={(value: number) => (
              <Tag bordered={false} color={value === 20 ? 'warning' : 'success'}>
                {paymentDirectionLabel(value)}
              </Tag>
            )}
          />
          <Table.Column
            title="往来单位"
            width={148}
            render={(_, row: ExchangeGainLine) => (
              <Space direction="vertical" size={0}>
                <span>{row.partner_name || <Empty />}</span>
                <Mono>{row.partner_code}</Mono>
              </Space>
            )}
          />
          <Table.Column
            dataIndex="currency_code"
            title="币种"
            width={68}
            render={(value: string) => <Tag>{value}</Tag>}
          />
          <Table.Column
            title="目标单据"
            width={160}
            // 单据链的另一半：损益是「到账汇率 − 这张单锁的汇率」，
            // 两个汇率分别在这边和那边，对不上时得能立刻跳过去看。
            render={(_, row: ExchangeGainLine) => {
              const path =
                row.target_type === 10
                  ? `/receivables/show/${row.target_id}`
                  : `/payables/show/${row.target_id}`
              return (
                <Space direction="vertical" size={0}>
                  <Link to={path}>
                    <Mono>{row.target_doc_no}</Mono>
                  </Link>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {PAYMENT_TARGET_OPTIONS.find((o) => o.value === row.target_type)?.label}
                  </Typography.Text>
                </Space>
              )
            }}
          />
          <Table.Column
            dataIndex="amount"
            title="核销原币"
            width={110}
            align="right"
            render={(value: string) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="payment_rate"
            title="到账汇率"
            width={100}
            align="right"
            render={(value: string) => (Number(value) === 1 ? '1' : Number(value).toFixed(4))}
          />
          <Table.Column
            dataIndex="document_rate"
            title="单据汇率"
            width={100}
            align="right"
            render={(value: string) => (Number(value) === 1 ? '1' : Number(value).toFixed(4))}
          />
          <Table.Column
            dataIndex="gain_base"
            title="损益（本币）"
            width={130}
            align="right"
            render={(value: string) => <GainValue value={value} strong />}
          />
        </ResponsiveTable>

        <InfiniteFooter feed={feed} />

        {data && data.subtotals.length > 0 && (
          <Space wrap size={8} style={{ marginTop: 12 }}>
            {data.subtotals.map((s) => (
              <Tag key={s.currency_code}>
                {s.currency_code}：核销 {Number(s.amount).toFixed(2)}，损益{' '}
                {Number(s.gain_base) > 0 ? '+' : ''}
                {Number(s.gain_base).toFixed(2)}
              </Tag>
            ))}
          </Space>
        )}

        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          口径：损益 = 核销原币 ×（到账汇率 − 单据锁定汇率）；应收到账汇率高记收益，应付付款汇率高记损失。
          预收 / 预付款还没核销，不产生损益。原币只按币种小计，跨币种只能加本币。
        </Typography.Paragraph>
      </Card>
    </Flex>
  )
}
