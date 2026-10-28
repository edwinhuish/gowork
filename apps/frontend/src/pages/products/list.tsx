import type { CrudFilters } from '@refinedev/core'
import { CreateButton, List, useTable } from '@refinedev/antd'
import { Button, Form, Input, Select, Space, Table } from 'antd'

import { CategoryTag, Empty, Mono, Money, StatusTag } from '@/components/fields'
import { CATEGORY_OPTIONS, STATUS_OPTIONS } from '@/constants/catalog'
import type { Product } from '@/types'

interface ProductSearch {
  keyword?: string
  category?: string
  status?: number
}

/**
 * 产品列表。
 *
 * 筛选走 refine 的 onSearch → CrudFilters → dataProvider 翻译成后端的
 * keyword / category / status 查询参数（见 providers/dataProvider.ts 的白名单）。
 * syncWithLocation 把分页与筛选写进 URL：从详情页返回时不用重新翻页和重填条件。
 */
export function ProductList() {
  const { tableProps, searchFormProps } = useTable<Product>({
    resource: 'products',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as ProductSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.category) {
        filters.push({ field: 'category', operator: 'eq', value: v.category })
      }
      if (v.status !== undefined && v.status !== null) {
        filters.push({ field: 'status', operator: 'eq', value: v.status })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <List headerButtons={<CreateButton>新建产品</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索 SKU / 中英文品名 / 规格" allowClear style={{ width: 260 }} />
        </Form.Item>
        <Form.Item name="category">
          <Select placeholder="全部类别" allowClear options={CATEGORY_OPTIONS} style={{ width: 160 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="全部状态" allowClear options={STATUS_OPTIONS} style={{ width: 120 }} />
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 1080 }}>
        <Table.Column
          dataIndex="sku_code"
          title="SKU 编码"
          width={150}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="name_cn"
          title="品名"
          render={(value: string, record: Product) => (
            <div className="cell-stack">
              <span className="cell-strong">{value}</span>
              {record.name_en && <span className="cell-sub">{record.name_en}</span>}
            </div>
          )}
        />
        <Table.Column
          dataIndex="category"
          title="类别"
          width={110}
          render={(value: string) => <CategoryTag value={value} />}
        />
        <Table.Column
          dataIndex="spec"
          title="规格"
          width={150}
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          dataIndex="supplier"
          title="供应商"
          width={150}
          render={(value: Product['supplier']) => value?.name || <Empty />}
        />
        <Table.Column
          dataIndex="cost_price"
          title="成本价"
          width={110}
          align="right"
          render={(value: number) => <Money value={value} />}
        />
        <Table.Column
          dataIndex="sale_price"
          title="售价"
          width={110}
          align="right"
          render={(value: number) => <Money value={value} />}
        />
        <Table.Column
          dataIndex="status"
          title="状态"
          width={90}
          render={(value: number) => <StatusTag value={value} />}
        />
      </Table>
    </List>
  )
}
