import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Select, Spin } from 'antd'

import { productApi } from '@/api/products'
import type { Product } from '@/types'

/**
 * 产品下拉里回显所需的最小信息。
 *
 * 编辑单据时行上只有 product_id 与当时的 SKU / 品名快照（产品后来改名也不该
 * 让旧单据变样），没有完整的产品对象。所以这里按"能显示得出来"取字段，
 * 售价是可选的——只有从搜索结果选中时才有，用来顺手带上单价。
 */
export interface ProductHint {
  id: number
  sku_code: string
  name_cn?: string
  /** 定点字符串：选中后可直接填进单价，不经过 number。 */
  sale_price?: string
}

interface ProductOption {
  value: number
  label: string
  hint: ProductHint
}

const DEBOUNCE_MS = 300
const PAGE_SIZE = 20

function toOption(p: ProductHint): ProductOption {
  return {
    value: p.id,
    label: p.name_cn ? `${p.sku_code} · ${p.name_cn}` : p.sku_code,
    hint: p,
  }
}

/**
 * 按关键词搜产品的下拉。
 *
 * 关键词走产品列表那一套（SKU / 中英文品名 / 各种编号），后端才会命中
 * 客户物料号、旧 SKU 与供应商货号——客户口头报的一个号，在这里就能找到货。
 *
 * 三个状态要说清：
 *   - hits 是这次输入搜回来的，随输入变；
 *   - selected 是当前选中的那条，必须常驻在 options 里，
 *     否则选中项在下次搜索后从列表消失，输入框只剩一个数字 id；
 *   - initialProduct 只在编辑回显时给，给了就不必再问服务端。
 */
export function ProductSelect({
  value,
  onChange,
  onPick,
  initialProduct,
  placeholder = '搜 SKU / 品名 / 客户编号',
  disabled,
}: {
  value?: number
  onChange?: (value: number | undefined) => void
  /** 选中了哪条产品：父组件用它回填单价、清掉不再适用的货源。 */
  onPick?: (product: ProductHint | undefined) => void
  /** 编辑回显用；没有也能工作，只是会多一次按 id 的查询。 */
  initialProduct?: ProductHint | null
  placeholder?: string
  disabled?: boolean
}) {
  const [hits, setHits] = useState<ProductOption[]>([])
  const [selected, setSelected] = useState<ProductOption | null>(null)
  const [loading, setLoading] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  // 请求序号：慢的那次回来时不能覆盖新关键词的结果，否则下拉里是上一次的内容。
  const seq = useRef(0)

  useEffect(() => {
    if (value === undefined || value === null) {
      setSelected(null)
      return
    }
    if (selected?.value === value) return

    if (initialProduct?.id === value) {
      setSelected(toOption(initialProduct))
      return
    }

    // 拿不到快照就按 id 补一条：否则输入框里显示的是一个光秃秃的数字。
    let alive = true
    productApi
      .get(value)
      .then((p) => {
        if (alive) setSelected(toOption(p))
      })
      .catch(() => {
        // 产品被删或没权限时保持空显示，不打断单据本身。
      })
    return () => {
      alive = false
    }
  }, [value, initialProduct, selected?.value])

  const search = useCallback((input: string) => {
    const keyword = input.trim()
    window.clearTimeout(timer.current)
    if (!keyword) {
      seq.current++
      setHits([])
      setLoading(false)
      return
    }

    timer.current = window.setTimeout(() => {
      const mine = ++seq.current
      setLoading(true)
      productApi
        .search(keyword, PAGE_SIZE)
        .then((res) => {
          if (mine === seq.current) setHits(res.list.map(toOption))
        })
        .catch(() => {
          if (mine === seq.current) setHits([])
        })
        .finally(() => {
          if (mine === seq.current) setLoading(false)
        })
    }, DEBOUNCE_MS)
  }, [])

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const options = useMemo(() => {
    const list = selected ? [selected, ...hits] : hits
    // 选中项同时也在搜索结果里时按 id 去重：antd 遇到重复 key 会报警并渲染错行。
    const seen = new Set<number>()
    return list.filter((o) => {
      if (seen.has(o.value)) return false
      seen.add(o.value)
      return true
    })
  }, [selected, hits])

  const handleChange = (next?: number) => {
    const picked = options.find((o) => o.value === next)
    if (picked) {
      // 选中的那条已经在手上，直接留作回显，免得 effect 再按 id 查一次。
      setSelected(picked)
    }
    if (next === undefined) {
      setSelected(null)
      setHits([])
    }
    onChange?.(next)
    onPick?.(picked?.hint)
  }

  return (
    <Select
      value={value}
      disabled={disabled}
      showSearch
      allowClear
      // 搜索交给后端：本地 filterOption 只能过滤已取到的那 20 条。
      filterOption={false}
      onSearch={search}
      onChange={handleChange}
      options={options}
      loading={loading}
      placeholder={placeholder}
      notFoundContent={loading ? <Spin size="small" /> : '没有匹配的产品'}
      optionRender={(option) => {
        const p = (option.data as ProductOption).hint as Partial<Product>
        const codes = p.matched_codes ?? []
        return (
          <div className="cell-stack">
            <span className="cell-strong">
              {p.sku_code}
              {p.name_cn ? ` · ${p.name_cn}` : ''}
            </span>
            {/* 按编号搜索时说明"为什么这条会出现"：SKU 与客户报的号对不上，
                不提示的话用户会以为搜错了。 */}
            {codes.length > 0 ? (
              <span className="cell-sub">命中编号：{codes.join('、')}</span>
            ) : p.name_en ? (
              <span className="cell-sub">{p.name_en}</span>
            ) : null}
          </div>
        )
      }}
    />
  )
}
