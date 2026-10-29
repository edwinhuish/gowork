import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Col, Row, Statistic, Typography } from 'antd'

import { reportApi } from '@/api/reports'
import { formatPercent } from '@/utils/format'

/**
 * 出货详情页的单票毛利面板。
 *
 * 毛利 = 收入 − 采购成本 − 运杂费（本币）。汇兑损益不在这里——T-23 单独成行，
 * 混进毛利会让同一笔差额在两张报表里各出现一次。
 *
 * 草稿出货不取数：数量还会改，算出来的毛利下一条编辑就作废。已取消的票
 * 后端也不会返回，面板此时与"没有分配行"共用一句说明。
 */
export function ProfitCard({ shipmentId, status }: { shipmentId: number; status: number }) {
  const isDraft = status === 10

  const { data, isLoading, error } = useQuery({
    queryKey: ['report-shipment-profit', shipmentId],
    queryFn: () => reportApi.shipmentProfits({ shipment_id: shipmentId }),
    enabled: !isDraft,
  })

  const profit = data?.list?.[0]

  return (
    <Card title="单票毛利" size="small" style={{ marginTop: 16 }} loading={isLoading}>
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      {isDraft && (
        <Typography.Text type="secondary">
          草稿出货不算毛利：数量还会改，确认之后再来看。
        </Typography.Text>
      )}

      {!isDraft && !isLoading && !profit && (
        <Typography.Text type="secondary">
          这票还没有可算的收入：已取消，或还没挂出货明细。
        </Typography.Text>
      )}

      {profit && (
        <>
          {/* T-36：成本与毛利在服务端就被清成了 0。这里必须显式说明，
              否则四个 0 会被读成"这票一分没赚"。 */}
          {profit.cost_hidden && (
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 12 }}
              message="成本与毛利对当前账号不可见"
              description="收入可以看到；成本、运杂费与毛利仅管理员与主管可见。"
            />
          )}

          {profit.cost_missing && !profit.cost_hidden && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message="缺少采购成本，毛利不完整"
              description="有销售行没有可用的采购单（或采购单仍是草稿），下面的成本与毛利只反映了已录入的那部分。"
            />
          )}

          <Row gutter={[16, 16]}>
            <Col xs={12} lg={6}>
              <Statistic title="收入（本币）" value={Number(profit.revenue)} precision={2} />
            </Col>
            <Col xs={12} lg={6}>
              <Statistic
                title="采购成本"
                value={profit.cost_hidden ? '—' : Number(profit.cost)}
                precision={profit.cost_hidden ? undefined : 2}
              />
            </Col>
            <Col xs={12} lg={6}>
              <Statistic
                title="运杂费"
                value={profit.cost_hidden ? '—' : Number(profit.charge)}
                precision={profit.cost_hidden ? undefined : 2}
              />
            </Col>
            <Col xs={12} lg={6}>
              <Statistic
                title="毛利"
                value={profit.cost_hidden ? '—' : Number(profit.profit)}
                precision={profit.cost_hidden ? undefined : 2}
                valueStyle={
                  !profit.cost_hidden && Number(profit.profit) < 0
                    ? { color: '#cf1322' }
                    : undefined
                }
              />
              <Typography.Text type="secondary" className="stat-hint">
                毛利率 {profit.cost_hidden ? '—' : formatPercent(Number(profit.margin))}
              </Typography.Text>
            </Col>
          </Row>

          <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
            口径：收入按本票数量分摊销售行的本币金额，成本按该行采购的加权单价 × 本票数量，
            两者都用单据上锁定的汇率。汇兑损益不并入毛利。
          </Typography.Paragraph>
        </>
      )}
    </Card>
  )
}
