import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Col, Row, Statistic, Typography } from 'antd'

import { dashboardApi } from '@/api/dashboard'

/**
 * 概览页。
 *
 * 这是聚合查询而不是 CRUD 资源，所以没走 dataProvider，直接用 react-query 调
 * dashboardApi。数据源只有一个接口，也没必要为它造一个 resource。
 *
 * 统计卡用 Row/Col 的 xs/sm/xl 断点做响应式：窄屏一行一个，宽屏一行四个，
 * 不写媒体查询。
 */
export function DashboardPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['dashboard-summary'],
    queryFn: dashboardApi.summary,
  })

  const stats = [
    { title: '产品总数', value: data?.product_total, hint: `在售 ${data?.active_products ?? '—'} 个` },
    { title: '供应商', value: data?.supplier_total, hint: '1688 店铺 / 工厂' },
    { title: '客户', value: data?.customer_total, hint: '进口商与终端门店' },
    { title: '待发货订单', value: 0, hint: '订单模块待接入' },
  ]

  return (
    <>
      {error && <Alert type="error" showIcon message={(error as Error).message} style={{ marginBottom: 16 }} />}

      <Row gutter={[16, 16]}>
        {stats.map((item) => (
          <Col key={item.title} xs={24} sm={12} xl={6}>
            <Card variant="borderless" className="stat-card">
              <Statistic title={item.title} value={item.value ?? '—'} loading={isLoading} />
              <Typography.Text type="secondary" className="stat-hint">
                {item.hint}
              </Typography.Text>
            </Card>
          </Col>
        ))}
      </Row>

      <Card variant="borderless" style={{ marginTop: 16 }} title="骨架说明">
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          当前已完成基础主数据（产品 / 供应商 / 客户）的完整分层实现，可作为后续模块的模板：
          新增业务模块时，按 <code>model → repository → service → handler → router</code> 的顺序各加一层，
          并在 <code>internal/bootstrap/app.go</code> 里完成装配即可。订单、采购、库存、出货批次等模块留待扩展。
        </Typography.Paragraph>
      </Card>
    </>
  )
}
