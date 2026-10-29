import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSelect } from '@refinedev/core'
import { Alert, Card, Input, Select, Space, Table, Tag, Typography } from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import { PAYMENT_TARGET_OPTIONS, paymentDirectionLabel } from '@/constants/catalog'
import type { Currency, ExchangeGainLine, Partner } from '@/types'

/**
 * 损益值：正绿负红。
 *
 * 符号不是装饰——这张表要回答"这个月汇率帮我赚了还是亏了"，
 * 而"−200"和"200"在数字列里长得几乎一样。
 */
function GainValue({ value, strong }: { value?: string | null; strong?: boolean }) {
  // 金额是定点字符串：这里的正负判断与色条只用于显示，取值本身原样交给 Money。
  if (value === null || value === undefined) return <Empty />
  const num = Number(value)
  if (Number.isNaN(num)) return <Empty />
  const color = num > 0.0001 ? '#389e0d' : num < -0.0001 ? '#cf1322' : undefined
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

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Card variant="borderless">
        <Space wrap size={12} align="center">
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
          <Typography.Text type="secondary">
            {range.from || range.to
              ? `按到账日 ${range.from || '不限'} ~ ${range.to || '不限'}`
              : '按到账日，默认全部'}
          </Typography.Text>
        </Space>
      </Card>

      <Card variant="borderless" title="汇兑损益明细">
        <Table<ExchangeGainLine>
          dataSource={lines}
          rowKey={(row) => `${row.payment_id}-${row.target_doc_no}-${row.amount}`}
          loading={isLoading}
          pagination={{ pageSize: 50, hideOnSinglePage: true }}
          scroll={{ x: 1300 }}
          locale={{ emptyText: '这段时间没有产生汇兑差额的核销' }}
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
            width={110}
            render={(value: string) => value}
          />
          <Table.Column
            dataIndex="payment_doc_no"
            title="收付款单"
            width={160}
            render={(value: string) => <Mono>{value}</Mono>}
          />
          <Table.Column
            dataIndex="direction"
            title="方向"
            width={80}
            render={(value: number) => (
              <Tag color={value === 20 ? 'orange' : 'green'}>{paymentDirectionLabel(value)}</Tag>
            )}
          />
          <Table.Column
            title="往来单位"
            width={180}
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
            width={80}
            render={(value: string) => <Tag>{value}</Tag>}
          />
          <Table.Column
            title="目标单据"
            width={190}
            render={(_, row: ExchangeGainLine) => (
              <Space direction="vertical" size={0}>
                <Mono>{row.target_doc_no}</Mono>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {PAYMENT_TARGET_OPTIONS.find((o) => o.value === row.target_type)?.label}
                </Typography.Text>
              </Space>
            )}
          />
          <Table.Column
            dataIndex="amount"
            title="核销原币"
            width={120}
            align="right"
            render={(value: string) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="payment_rate"
            title="到账汇率"
            width={110}
            align="right"
            render={(value: string) => (Number(value) === 1 ? '1' : Number(value).toFixed(4))}
          />
          <Table.Column
            dataIndex="document_rate"
            title="单据汇率"
            width={110}
            align="right"
            render={(value: string) => (Number(value) === 1 ? '1' : Number(value).toFixed(4))}
          />
          <Table.Column
            dataIndex="gain_base"
            title="汇兑损益（本币）"
            width={150}
            align="right"
            render={(value: string) => <GainValue value={value} strong />}
          />
        </Table>

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
    </Space>
  )
}
