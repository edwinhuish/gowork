/**
 * 产品类别字典。
 *
 * 与后端 model.SkuCode 的大类约定保持一致（SKU 前两位即类别码）。
 * 后端目前不校验类别合法性，所以这里是唯一事实来源 —— 改动要同时看
 * apps/backend/internal/model/product.go 的注释。
 */
export const PRODUCT_CATEGORIES = [
  { code: 'HT', label: '手动工具' },
  { code: 'PT', label: '电动工具' },
  { code: 'FS', label: '紧固件' },
  { code: 'BH', label: '建筑五金' },
  { code: 'LK', label: '锁具' },
  { code: 'PL', label: '水暖管件' },
  { code: 'MS', label: '测量工具' },
  { code: 'AB', label: '磨料耗材' },
  { code: 'EL', label: '电气' },
  { code: 'PK', label: '包装辅料' },
] as const

const CATEGORY_MAP = new Map<string, string>(
  PRODUCT_CATEGORIES.map((c) => [c.code, c.label] as [string, string]),
)

export function categoryLabel(code?: string | null): string | undefined {
  if (!code) return undefined
  return CATEGORY_MAP.get(code) ?? code
}

/** 类别下拉选项，统一带「未分类」空项。 */
export const CATEGORY_OPTIONS = PRODUCT_CATEGORIES.map((c) => ({
  value: c.code,
  label: `${c.code} · ${c.label}`,
}))

/** 启用/停用下拉选项。后端用 int8（1 启用 / 0 停用）。 */
export const STATUS_OPTIONS = [
  { value: 1, label: '启用' },
  { value: 0, label: '停用' },
]
