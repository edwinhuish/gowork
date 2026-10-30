import { useInvalidate } from '@refinedev/core'
import { EditButton, ShowButton, useTable } from '@refinedev/antd'
import { Button, message, Popconfirm, Space, Table, Tag } from 'antd'

import { paymentApi } from '@/api/payments'
import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { PAYMENT_DIRECTION_OPTIONS } from '@/constants/catalog'
import type { Payment } from '@/types'
import { formatDate } from '@/utils/format'

/**
 * 收付款列表。
 *
 * 默认按到账日倒序（后端排的）：这张表的读法是「最近的钱都去哪了」，
 * 与应收应付的「哪笔先到期」是两种读法。
 *
 * 「已核销」由核销行求和而来，不另取字段：核销行只增不改，
 * 两者永远不该出现两个数。
 */
export function PaymentList() {
  const invalidate = useInvalidate()
  const { tableProps } = useTable<Payment>({ resource: 'payments', syncWithLocation: true })

  const remove = async (record: Payment) => {
    try {
      await paymentApi.remove(record.id)
      message.success(`已删除 ${record.doc_no}`)
      await invalidate({ resource: 'payments', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <PageList
      createLabel="登记收付款"
      tableProps={tableProps}
      rowKey="id"
      scrollX={1200}
    >
      <Table.Column
        dataIndex="doc_no"
        title="单号"
        width={160}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      <Table.Column
        dataIndex="direction"
        title="方向"
        width={90}
        render={(value: number) => <OptionTag value={value} options={PAYMENT_DIRECTION_OPTIONS} />}
      />
      <Table.Column
        dataIndex="partner"
        title="往来单位"
        width={180}
        render={(value: Payment['partner']) =>
          value ? <span className="cell-strong">{value.name}</span> : <Empty />
        }
      />
      <Table.Column
        dataIndex="pay_date"
        title="到账日"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="amount"
        title="金额"
        width={170}
        align="right"
        // 币种交给 Money：它按该币种的小数位显示（JPY 0 位、KWD 3 位），
        // 并在金额后面带上代码。之前固定 2 位 + 单独的 Tag，
        // 日元会显示成 1,000.00、科威特第纳尔会丢第三位。
        render={(value: string, record: Payment) => (
          <Money value={value} currency={record.currency_code} />
        )}
      />
      <Table.Column
        dataIndex="exchange_rate"
        title="汇率"
        width={100}
        align="right"
        render={(value: string) => (Number(value) === 1 ? '1' : Number(value).toFixed(4))}
      />
      <Table.Column
        title="已核销"
        width={130}
        align="right"
        // 已核销与未核销只用于列表提示，算术在服务端按末位做。
        render={(_, record: Payment) => {
          const allocated = (record.allocations ?? []).reduce((sum, a) => sum + Number(a.amount), 0)
          if (allocated <= 0) {
            return <Tag bordered={false}>{record.direction === 20 ? '预付' : '预收'}</Tag>
          }
          return <Money value={allocated} currency={record.currency_code} />
        }}
      />
      <Table.Column
        title="未核销"
        width={120}
        align="right"
        render={(_, record: Payment) => {
          const allocated = (record.allocations ?? []).reduce((sum, a) => sum + Number(a.amount), 0)
          const rest = Number(record.amount) - allocated
          return rest > 0.0001 ? <Money value={rest} currency={record.currency_code} /> : <Empty />
        }}
      />
      <Table.Column
        dataIndex="reference"
        title="水单号"
        width={150}
        render={(value: string) => (value ? <Mono>{value}</Mono> : <Empty />)}
      />
      <Table.Column
        title="操作"
        width={180}
        fixed="right"
        render={(_, record: Payment) => (
          <Space size={4}>
            <ShowButton size="small" type="link" recordItemId={record.id}>
              详情
            </ShowButton>
            <EditButton size="small" type="link" recordItemId={record.id}>
              编辑
            </EditButton>
            <Popconfirm
              title="删除这张单？"
              description="目标单据的已核销金额与状态会一起回滚（核销行不保留）"
              okText="删除"
              cancelText="取消"
              onConfirm={() => remove(record)}
            >
              <Button size="small" type="link" danger>
                删除
              </Button>
            </Popconfirm>
          </Space>
        )}
      />
    </PageList>
  )
}
