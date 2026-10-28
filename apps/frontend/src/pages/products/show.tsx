import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions } from 'antd'

import { CategoryTag, Empty, Money, StatusTag } from '@/components/fields'
import type { Product } from '@/types'
import { formatTime } from '@/utils/format'

import { SourcesPanel } from './source-panel'

/**
 * 产品详情。
 *
 * 改造前没有详情页，"看一条"和"改一条"混在弹窗里：只想核对信息也得进编辑态。
 * 详情页把只读核对与写入分开，也给了后续挂子表（编码历史、报价行、出货批次）的位置。
 *
 * 列宽用 antd 的响应式对象，窄屏自动降为单列，不靠手写媒体查询。
 */
export function ProductShow() {
  const { result, query } = useShow<Product>({ resource: 'products' })

  return (
    <Show isLoading={query.isLoading} title={result?.name_cn ?? '产品详情'}>
      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="middle"
        items={[
          {
            key: 'sku_code',
            label: 'SKU 编码',
            children: <span className="cell-mono">{result?.sku_code}</span>,
          },
          { key: 'category', label: '类别', children: <CategoryTag value={result?.category} /> },
          { key: 'status', label: '状态', children: <StatusTag value={result?.status} /> },
          { key: 'name_cn', label: '中文品名', children: result?.name_cn || <Empty /> },
          { key: 'name_en', label: '英文品名', children: result?.name_en || <Empty /> },
          { key: 'spec', label: '规格', children: result?.spec || <Empty /> },
          {
            key: 'supplier',
            label: '供应商',
            children: result?.supplier?.name || <Empty />,
          },
          { key: 'unit', label: '单位', children: result?.unit || <Empty /> },
          {
            key: 'cost_price',
            label: '成本价',
            children: <Money value={result?.cost_price} />,
          },
          {
            key: 'sale_price',
            label: '售价',
            children: <Money value={result?.sale_price} />,
          },
          {
            key: 'remark',
            label: '备注',
            span: 2,
            children: result?.remark || <Empty />,
          },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      {/* 货源是产品的子表：报价成本从这里取，产品页上没有它就等于成本只能手填。 */}
      {result?.id ? <SourcesPanel productId={result.id} /> : null}
    </Show>
  )
}
