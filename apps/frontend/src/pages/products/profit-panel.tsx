import { useQuery } from '@tanstack/react-query'
import { Card, Space, Table, Tag, Typography } from 'antd'
import { useNavigate } from 'react-router'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import type { ProductProfitRow } from '@/types'

/**
 * 产品详情的「逐票毛利」（T-31 下钻）。
 *
 * 产品详情原来只讲这个货是什么，不回答问题"卖它赚不赚钱、赚在哪几票"。
 * 这里按**出货票**列出收入 / 成本 / 运杂费 / 毛利，每行都能点进那一票：
 * 毛利对不上时，下一步要看的一定是那一票的费用与装箱，而不是这个产品本身。
 *
 * 成本口径在服务端（与单票毛利、销售看板同源），前端不重算；
 * 被 T-36 隐藏成本时显示「—」而不是 0——0 会被读成"没赚钱"。
 */
export function ProfitPanel({ productId }: { productId: number }) {
  const navigate = useNavigate()
  const { data, isLoading, error } = useQuery({
    queryKey: ['product-profit', productId],
    queryFn: () => reportApi.productProfit({ product_id: productId }),
  })

  const hidden = data?.cost_hidden === true

  return (
    <Card
      title="逐票毛利"
      size="small"
      style={{ marginTop: 16 }}
      loading={isLoading}
      extra={
        <Space size={12}>
          <span className="cell-sub">
            出货 {data?.rows.length ?? 0} 票；金额均为本币
          </span>
        </Space>
      }
    >
      {error ? (
        <Typography.Text type="danger">{(error as Error).message}</Typography.Text>
      ) : (
        <>
          <Table<ProductProfitRow>
            rowKey="shipment_id"
            size="small"
            pagination={false}
            scroll={{ x: 860 }}
            dataSource={data?.rows ?? []}
            locale={{ emptyText: '这个产品还没有出货记录' }}
          >
            <Table.Column
              dataIndex="ship_date"
              title="出货日"
              width={110}
            />
            <Table.Column
              dataIndex="doc_no"
              title="出货单"
              width={170}
              render={(value: string, row: ProductProfitRow) => (
                <Typography.Link onClick={() => navigate(`/shipments/show/${row.shipment_id}`)}>
                  <Mono>{value}</Mono>
                </Typography.Link>
              )}
            />
            <Table.Column
              dataIndex="partner_name"
              title="客户"
              render={(value: string, row: ProductProfitRow) =>
                value ? (
                  <Typography.Link onClick={() => navigate(`/customers/show/${row.partner_id}`)}>
                    {value}
                  </Typography.Link>
                ) : (
                  <Empty />
                )
              }
            />
            <Table.Column
              dataIndex="qty"
              title="数量"
              width={90}
              align="right"
              render={(value: string) => Number(value).toLocaleString('zh-CN')}
            />
            <Table.Column
              dataIndex="revenue"
              title="收入"
              width={120}
              align="right"
              render={(value: string) => <Money value={value} />}
            />
            <Table.Column
              dataIndex="cost"
              title="采购成本"
              width={120}
              align="right"
              render={(value: string, row: ProductProfitRow) =>
                row.cost_hidden || row.cost_missing ? <span className="cell-empty">—</span> : <Money value={value} />
              }
            />
            <Table.Column
              dataIndex="charge"
              title="运杂费"
              width={110}
              align="right"
              render={(value: string, row: ProductProfitRow) =>
                row.cost_hidden ? <span className="cell-empty">—</span> : <Money value={value} />
              }
            />
            <Table.Column
              dataIndex="profit"
              title="毛利"
              width={130}
              align="right"
              render={(value: string, row: ProductProfitRow) =>
                row.cost_hidden || row.cost_missing ? <span className="cell-empty">—</span> : <Money value={value} />
              }
            />
          </Table>

          <Space size={16} wrap style={{ marginTop: 12 }}>
            <span className="cell-sub">
              合计收入 <Money value={data?.revenue} />
            </span>
            <span className="cell-sub">
              合计毛利{' '}
              {hidden ? <span className="cell-empty">—</span> : <Money value={data?.profit} />}
            </span>
            <span className="cell-sub">
              毛利率{' '}
              {hidden ? (
                <span className="cell-empty">—</span>
              ) : (
                `${(Number(data?.margin ?? 0) * 100).toFixed(1)}%`
              )}
            </span>
            {data?.cost_missing ? (
              <Tag color="warning" bordered={false}>
                部分出货缺采购成本，毛利不完整
              </Tag>
            ) : null}
            {hidden ? (
              <Tag bordered={false}>当前角色看不到成本类金额</Tag>
            ) : null}
          </Space>
        </>
      )}
    </Card>
  )
}
