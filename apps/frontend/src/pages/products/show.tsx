import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions } from 'antd'

import { AttachmentPanel } from '@/components/attachment-panel'
import { CategoryTag, Empty, Money, StatusTag } from '@/components/fields'
import type { Product } from '@/types'
import { formatTime } from '@/utils/format'

import { CodePanel } from './code-panel'
import { ProductImagesPanel } from './image-panel'
import { ProfitPanel } from './profit-panel'
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
      {/* 图片放在最前：客户来问「这件货长什么样」时，先看的不是 HS 编码。
          与货源/编号一致做成子表面板，产品表单不参与图片。 */}
      {result?.id ? <ProductImagesPanel productId={result.id} /> : null}

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
            // T-36：成本价被服务端清过时显示「—」而不是 0——
            // 「看不见」与「这件货不要钱」是两件事。
            children: result?.cost_hidden ? (
              <span className="cell-sub">—（无权限）</span>
            ) : (
              <Money value={result?.cost_price} />
            ),
          },
          {
            key: 'sale_price',
            label: '售价',
            children: <Money value={result?.sale_price} />,
          },
          { key: 'hs_code', label: 'HS 编码', children: result?.hs_code || <Empty /> },
          { key: 'legal_unit', label: '法定单位', children: result?.legal_unit || <Empty /> },
          {
            key: 'declared_name_cn',
            label: '申报品名（中文）',
            children: result?.declared_name_cn || <Empty />,
          },
          {
            key: 'declared_name_en',
            label: '申报品名（英文）',
            children: result?.declared_name_en || <Empty />,
          },
          { key: 'origin_country', label: '原产地', children: result?.origin_country || <Empty /> },
          {
            key: 'material',
            label: '材质与用途',
            span: 2,
            children: result?.material || <Empty />,
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

      {/* 编号放在货源之前：客户报一个号来问价时，先要确认「这号是不是这件货」。 */}
      {result?.id ? <CodePanel productId={result.id} /> : null}

      {/* 货源是产品的子表：报价成本从这里取，产品页上没有它就等于成本只能手填。 */}
      {result?.id ? <SourcesPanel productId={result.id} /> : null}

      {/* 卖它赚不赚钱、赚在哪几票：只讲「这件货是什么」的产品页回答不了这个问题。 */}
      {result?.id ? <ProfitPanel productId={result.id} /> : null}

      {/* 图纸、规格书、检测报告这类跟着产品走的文件：它们多年不变，
          挂在产品上比挂在某张订单上更容易找到——找的是"这件货长什么样"。 */}
      {result?.id ? (
        <AttachmentPanel entityType="product" entityId={result.id} title="图纸与检测报告" />
      ) : null}
    </Show>
  )
}
