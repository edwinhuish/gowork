import { useList } from '@refinedev/core'
import { Select } from 'antd'

import type { ProductSource } from '@/types'

/**
 * 明细行的成本货源下拉。
 *
 * 按产品现查，不一次取满全量：货源条数随供应商数量增长，一个产品挂五六家很常见，
 * 而全量取数的上限是 200 条——超了就会静默丢项，表现是"这家明明有报价，下拉里没有"。
 *
 * 只列启用的货源：停用的货源仍会留在历史单据里，但新报价不该再选它。
 * 已选的那条若已停用或被删，仍然显示出来（标成"已停用或已删除"），
 * 否则输入框里只剩一个数字 id，看不出这行锁的是哪家的成本。
 */
export function SourceSelect({
  productId,
  value,
  onChange,
  disabled,
}: {
  productId?: number
  value?: number
  onChange?: (value: number | undefined) => void
  disabled?: boolean
}) {
  const { query } = useList<ProductSource>({
    resource: 'product-sources',
    pagination: { mode: 'off' },
    filters: productId
      ? [
          { field: 'product_id', operator: 'eq', value: productId },
          { field: 'status', operator: 'eq', value: 1 },
        ]
      : [],
    queryOptions: { enabled: !!productId },
  })

  const sources = query.data?.data ?? []

  const options = sources.map((s) => ({
    value: s.id,
    label: [
      s.partner?.name ?? '供应商',
      Number(s.purchase_price_base).toFixed(2),
      s.supplier_sku,
    ]
      .filter(Boolean)
      .join(' · '),
  }))

  // 选中的那条不在列表里：补一行占位，避免下拉显示成一个光秃秃的 id。
  if (value !== undefined && !options.some((o) => o.value === value)) {
    options.unshift({ value, label: `货源 #${value}（已停用或已删除）` })
  }

  return (
    <Select
      value={value}
      onChange={onChange}
      allowClear
      disabled={disabled || !productId}
      loading={query.isLoading}
      options={options}
      placeholder={
        !productId ? '先选产品' : sources.length === 0 ? '该产品还没有货源' : '最低价（不选则自动取）'
      }
    />
  )
}
