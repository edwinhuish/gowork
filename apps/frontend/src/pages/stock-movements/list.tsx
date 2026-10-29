import type { CrudFilters } from '@refinedev/core'
import {
  CreateButton,
  DeleteButton,
  List,
  useTable,
} from '@refinedev/antd'
import { Button, Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono } from '@/components/fields'
import {
  MANUAL_STOCK_MOVEMENT_OPTIONS,
  STOCK_MOVEMENT_OPTIONS,
  stockMovementLabel,
} from '@/constants/catalog'
import type { StockMovement } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

interface MovementSearch {
  keyword?: string
  type?: number
}

/** 只有手工录入的流水能删；单据产生的要回到单据上处理。 */
function isManual(type: number): boolean {
  return MANUAL_STOCK_MOVEMENT_OPTIONS.some((o) => o.value === type)
}

export function StockMovementList() {
  const { tableProps, searchFormProps } = useTable<StockMovement>({
    resource: 'stock-movements',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as MovementSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.type) {
        filters.push({ field: 'type', operator: 'eq', value: v.type })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <List headerButtons={<CreateButton>手工录入</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索单号 / SKU / 品名 / 备注" allowClear style={{ width: 260 }} />
        </Form.Item>
        <Form.Item name="type">
          <Select
            placeholder="全部类型"
            allowClear
            style={{ width: 140 }}
            options={STOCK_MOVEMENT_OPTIONS}
          />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">
              查询
            </Button>
            <Button onClick={resetFilters}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <Table {...tableProps} rowKey="id" scroll={{ x: 1000 }}>
        <Table.Column
          dataIndex="doc_no"
          title="流水号"
          width={160}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="occurred_at"
          title="发生日"
          width={120}
          render={(value: string) => formatDate(value)}
        />
        <Table.Column
          dataIndex="sku_code"
          title="SKU"
          width={140}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="name_cn"
          title="品名 / 规格"
          render={(value: string, record: StockMovement) => (
            <div className="cell-stack">
              <span className="cell-strong">{value || <Empty />}</span>
              {record.spec && <span className="cell-sub">{record.spec}</span>}
            </div>
          )}
        />
        <Table.Column
          dataIndex="type"
          title="类型"
          width={110}
          render={(value: number) => <Tag>{stockMovementLabel(value)}</Tag>}
        />
        <Table.Column
          dataIndex="qty"
          title="数量"
          width={110}
          align="right"
          render={(value: string, record: StockMovement) => (
            <span className="cell-mono">
              {record.direction === -1 ? '−' : '+'}
              {formatQty(value)}
              {record.unit ? ` ${record.unit}` : ''}
            </span>
          )}
        />
        <Table.Column
          dataIndex="remark"
          title="备注 / 来源"
          render={(value: string, record: StockMovement) => {
            const source = record.source_doc_no
            return (
              <div className="cell-stack">
                <span>{value || <Empty />}</span>
                {source && <span className="cell-sub">来源 {source}</span>}
              </div>
            )
          }}
        />
        <Table.Column
          title="操作"
          width={90}
          render={(_, record: StockMovement) =>
            isManual(record.type) ? (
              <DeleteButton
                size="small"
                resource="stock-movements"
                recordItemId={record.id}
                confirmTitle="删除这条库存流水？"
                confirmOkText="删除"
                confirmCancelText="返回"
              />
            ) : (
              <span className="cell-sub">由单据产生</span>
            )
          }
        />
      </Table>
    </List>
  )
}
