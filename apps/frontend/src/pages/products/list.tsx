import { AppstoreOutlined, BarsOutlined, PictureOutlined } from '@ant-design/icons'
import { useInvalidate, useNavigation, type CrudFilters, type CrudSort } from '@refinedev/core'
import { CreateButton, DeleteButton, List, useTable } from '@refinedev/antd'
import {
  Button,
  Checkbox,
  Empty,
  Form,
  Input,
  message,
  Pagination,
  Popconfirm,
  Segmented,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
  Tooltip,
} from 'antd'

import { exportApi } from '@/api/exports'
import { productApi } from '@/api/products'
import { AuthedImage } from '@/components/authed-image'
import { CategoryTag, Empty as EmptyValue, Mono, Money, StatusTag } from '@/components/fields'
import { CATEGORY_OPTIONS, ENABLE_STATUS_OPTIONS } from '@/constants/catalog'
import { useListView } from '@/hooks/use-list-view'
import type { Product } from '@/types'

interface ProductSearch {
  keyword?: string
  category?: string
  status?: number
  /** 含已删除：打开后已软删的行也出现在列表里，只能做一件事——恢复。 */
  include_deleted?: boolean
}

/**
 * 排序选项。
 *
 * 只列后端白名单里真实存在的字段（repository.sortableProductFields），
 * 多列一个就是"选了没反应"。成本价不在其中：它对业务员是隐藏字段，
 * 按看不见的数字排序，结果无法解释。
 */
const SORT_OPTIONS = [
  { value: 'default', label: '默认顺序' },
  { value: 'sku_code:asc', label: '编码 A → Z' },
  { value: 'sku_code:desc', label: '编码 Z → A' },
  { value: 'name_cn:asc', label: '品名 A → Z' },
  { value: 'sale_price:desc', label: '售价 高 → 低' },
  { value: 'sale_price:asc', label: '售价 低 → 高' },
]

const PAGE_SIZE_OPTIONS = ['12', '24', '48']

/** 导出产品（成本价按 T-36 的权限留空）。失败时给一句人话。 */
async function exportProducts() {
  try {
    await exportApi.products()
  } catch (err) {
    message.error(err instanceof Error ? err.message : '导出失败')
  }
}

/**
 * 产品列表：表格视图 + 卡片视图。
 *
 * 筛选走 refine 的 onSearch → CrudFilters → dataProvider 翻译成后端的
 * keyword / category / status / sort_field / sort_order（白名单见 dataProvider）。
 * syncWithLocation 把分页、筛选、排序写进 URL：从详情页返回时不用重新翻页和重填条件。
 *
 * 两种视图共用同一个 useTable：切换只换渲染方式，不换数据源。
 * 否则每切一次都要重新请求、页码与筛选还会丢。
 */
export function ProductList() {
  const { view, setView } = useListView('gowork:products:view')
  const { show } = useNavigation()
  const invalidate = useInvalidate()

  const { tableProps, searchFormProps, sorters, setSorters } = useTable<Product>({
    resource: 'products',
    syncWithLocation: true,
    pagination: { pageSize: 12 },
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
      if (v.include_deleted) {
        filters.push({ field: 'include_deleted', operator: 'eq', value: true })
      }
      return filters
    },
  })

  const rows = (tableProps.dataSource ?? []) as Product[]
  const activeSort = sorters?.[0]
  // 排序状态的唯一真相是 useTable 的 sorters：表格点表头与下拉选择都写回它。
  const sortValue = activeSort ? `${activeSort.field}:${activeSort.order}` : 'default'

  const changeSort = (value: string) => {
    if (value === 'default') {
      setSorters([])
      return
    }
    const [field, order] = value.split(':')
    setSorters([{ field, order: order === 'desc' ? 'desc' : 'asc' }] as CrudSort[])
  }

  /** 表头箭头跟随同一个排序状态，点表头与用下拉框是同一件事。 */
  const sortOrderOf = (field: string): 'ascend' | 'descend' | null => {
    if (activeSort?.field !== field) return null
    return activeSort.order === 'desc' ? 'descend' : 'ascend'
  }

  const restore = async (record: Product) => {
    try {
      await productApi.restore(record.id)
      message.success(`${record.sku_code} 已恢复`)
      await invalidate({ resource: 'products', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  // 表格与卡片共用一份分页配置：卡片视图没有表格自带的分页条，要自己渲染。
  const pager = tableProps.pagination === false ? undefined : tableProps.pagination
  const cardPager = pager
    ? {
        current: pager.current ?? 1,
        pageSize: pager.pageSize ?? 12,
        total: pager.total ?? 0,
        showSizeChanger: true,
        pageSizeOptions: PAGE_SIZE_OPTIONS,
        showTotal: (total: number) => `共 ${total} 个产品`,
        onChange: pager.onChange as (page: number, pageSize: number) => void,
      }
    : undefined

  return (
    <List
      headerButtons={
        <Space wrap size={8}>
          <Segmented
            value={view}
            onChange={(value) => setView(value as 'list' | 'card')}
            options={[
              { value: 'list', icon: <BarsOutlined />, title: '列表视图：一行一件，字段最全' },
              { value: 'card', icon: <AppstoreOutlined />, title: '卡片视图：按图找货' },
            ]}
            aria-label="切换产品视图"
          />
          <Select
            value={sortValue}
            onChange={changeSort}
            options={SORT_OPTIONS}
            style={{ width: 148 }}
            aria-label="排序方式"
          />
          <CreateButton>新建产品</CreateButton>
          <Tooltip title="导出全部产品（CSV，忽略当前筛选，上限 10000 行；成本价按权限留空）">
            <Button onClick={() => void exportProducts()}>导出 CSV</Button>
          </Tooltip>
        </Space>
      }
    >
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索 SKU / 中英文品名 / 规格" allowClear style={{ width: 260 }} />
        </Form.Item>
        <Form.Item name="category">
          <Select placeholder="全部类别" allowClear options={CATEGORY_OPTIONS} style={{ width: 160 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="全部状态" allowClear options={ENABLE_STATUS_OPTIONS} style={{ width: 120 }} />
        </Form.Item>
        <Form.Item name="include_deleted" valuePropName="checked">
          <Checkbox>含已删除</Checkbox>
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

      {view === 'list' ? (
        <Table {...tableProps} rowKey="id" scroll={{ x: 1150 }} {...(pager ? { pagination: pager } : {})}>
          {/* 主图缩略图：列表只显示顺序第一张，没有就留占位。
              后端已经整页批量带出 primary_image，不会逐行再请求。 */}
          <Table.Column
            dataIndex="primary_image"
            title="图片"
            width={72}
            render={(value: Product['primary_image'], record: Product) =>
              value ? (
                <AuthedImage
                  path={value.thumb_url}
                  alt={record.name_cn || record.sku_code}
                  style={{
                    width: 48,
                    height: 48,
                    objectFit: 'cover',
                    borderRadius: 6,
                    display: 'block',
                    background: '#fafafa',
                  }}
                />
              ) : (
                <span
                  className="cell-sub"
                  title="未上传图片"
                  style={{ display: 'inline-block', fontSize: 20, lineHeight: 1 }}
                >
                  <PictureOutlined />
                </span>
              )
            }
          />
          <Table.Column
            dataIndex="sku_code"
            title="SKU 编码"
            width={170}
            sorter
            sortOrder={sortOrderOf('sku_code')}
            render={(value: string, record: Product) => (
              <div className="cell-stack">
                <Space size={4}>
                  <Mono>{value}</Mono>
                  {record.deleted_at ? <Tag bordered={false}>已删除</Tag> : null}
                </Space>
                {/* 搜旧编号时说明"为什么这条会出现"；不说明的话，用户看着一串
                    对不上的 SKU 会以为搜索坏了。 */}
                {record.matched_codes?.length ? (
                  <span className="cell-sub">命中编号：{record.matched_codes.join('、')}</span>
                ) : null}
              </div>
            )}
          />
          <Table.Column
            dataIndex="name_cn"
            title="品名"
            sorter
            sortOrder={sortOrderOf('name_cn')}
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
            render={(value: string) => value || <EmptyValue />}
          />
          <Table.Column
            dataIndex="supplier"
            title="供应商"
            width={150}
            render={(value: Product['supplier']) => value?.name || <EmptyValue />}
          />
          <Table.Column
            dataIndex="cost_price"
            title="成本价"
            width={110}
            align="right"
            // T-36：被服务端清过的成本价显示「—」，0 会被读成"这件货不要钱"。
            render={(value: string, row: Product) =>
              row.cost_hidden ? <span className="cell-sub">—</span> : <Money value={value} />
            }
          />
          <Table.Column
            dataIndex="sale_price"
            title="售价"
            width={110}
            align="right"
            sorter
            sortOrder={sortOrderOf('sale_price')}
            render={(value: number) => <Money value={value} />}
          />
          <Table.Column
            dataIndex="status"
            title="状态"
            width={90}
            render={(value: number) => <StatusTag value={value} />}
          />
          <Table.Column
            title="操作"
            width={110}
            render={(_, record: Product) => (
              <RowActions record={record} onRestore={restore} />
            )}
          />
        </Table>
      ) : (
        <>
          {tableProps.loading && rows.length === 0 ? (
            <div className="product-grid">
              {Array.from({ length: 8 }).map((_, i) => (
                <div className="product-card product-card--ghost" key={i}>
                  <Skeleton.Image active className="product-card__ghost-media" />
                  <Skeleton active paragraph={{ rows: 2 }} title={false} />
                </div>
              ))}
            </div>
          ) : rows.length === 0 ? (
            <Empty description="没有符合条件的产品" image={Empty.PRESENTED_IMAGE_SIMPLE}>
              <Space>
                <Button onClick={resetFilters}>重置筛选</Button>
                <CreateButton>新建产品</CreateButton>
              </Space>
            </Empty>
          ) : (
            <>
              <div className="product-grid">
                {rows.map((record) => (
                  <article
                    key={record.id}
                    className="product-card"
                    role="button"
                    tabIndex={0}
                    onClick={() => show('products', record.id)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault()
                        show('products', record.id)
                      }
                    }}
                  >
                    <div className="product-card-media">
                      {record.primary_image ? (
                        <AuthedImage
                          path={record.primary_image.thumb_url}
                          alt={record.name_cn || record.sku_code}
                          className="product-card-img"
                        />
                      ) : (
                        <span className="product-card-placeholder">
                          <PictureOutlined />
                        </span>
                      )}
                      <div className="product-card-badges">
                        <CategoryTag value={record.category} />
                        {record.deleted_at ? <Tag bordered={false}>已删除</Tag> : null}
                      </div>
                    </div>

                    <div className="product-card-body">
                      <h3 className="product-card-title" title={record.name_cn}>
                        {record.name_cn}
                      </h3>
                      <p className="product-card-sub" title={record.name_en || record.sku_code}>
                        {record.name_en || '—'}
                      </p>

                      <div className="product-card-meta">
                        <Mono>{record.sku_code}</Mono>
                        <span className="cell-sub">{record.spec || '未填规格'}</span>
                      </div>

                      <div className="product-card-prices">
                        <div className="product-card-price">
                          <span className="cell-sub">售价</span>
                          <Money value={record.sale_price} />
                        </div>
                        <div className="product-card-price">
                          <span className="cell-sub">成本</span>
                          {record.cost_hidden ? (
                            <span className="cell-sub">—</span>
                          ) : (
                            <Money value={record.cost_price} />
                          )}
                        </div>
                      </div>

                      <div className="product-card-footer">
                        <span className="cell-sub" title={record.supplier?.name || ''}>
                          {record.supplier?.name || '未指定供应商'}
                        </span>
                        <span
                          className="product-card-actions"
                          onClick={(event) => event.stopPropagation()}
                          onKeyDown={(event) => event.stopPropagation()}
                        >
                          <StatusTag value={record.status} />
                          <RowActions record={record} onRestore={restore} />
                        </span>
                      </div>
                    </div>
                  </article>
                ))}
              </div>
              {cardPager && (
                <div className="product-grid-pager">
                  <Pagination {...cardPager} />
                </div>
              )}
            </>
          )}
        </>
      )}
    </List>
  )
}

/**
 * 行/卡片右下角的动作。
 *
 * 已删除的行只能恢复：编辑与删除对它们都是 404，按钮摆出来也没用。
 */
function RowActions({ record, onRestore }: { record: Product; onRestore: (record: Product) => Promise<void> }) {
  if (record.deleted_at) {
    return (
      <Popconfirm
        title="恢复这个产品？"
        description="只去掉删除标记，删除时清掉的编号不会回来。"
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
      resource="products"
      recordItemId={record.id}
      confirmTitle="删除这个产品？"
      confirmOkText="删除"
      confirmCancelText="返回"
    />
  )
}
