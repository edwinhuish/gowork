/**
 * 产品类别字典。
 *
 * 与后端 model.Product.SKUCode 的大类约定保持一致（SKU 前两位即类别码）。
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

/** 启用状态下拉选项。取值与后端 model.EnableStatus 对齐：1 启用 / 0 停用。 */
export const ENABLE_STATUS_OPTIONS = [
  { value: 1, label: '启用' },
  { value: 0, label: '停用' },
]

/**
 * 主体类型选项。0 公司 / 1 个人，与后端 model.PartnerEntityType 对齐。
 *
 * 只有两项，用单选按钮而不是下拉：下拉要在两次点击之后才能看到全部选项，
 * 而这一项决定了报关资料按公司还是个人走，值得直接摊开。
 */
export const PARTNER_ENTITY_TYPE_OPTIONS = [
  { value: 0, label: '公司' },
  { value: 1, label: '个人' },
]

/**
 * 主体角色位。1 客户 / 2 供应商，两者兼具时是 3。
 *
 * 这里是「位」而不是「选项」：角色可以叠加，所以没有值为 3 的第三个选项，
 * 界面上的多选由位运算合成，展示由 RoleTag 拆开。
 */
export const PARTNER_ROLE = {
  customer: 1,
  supplier: 2,
} as const
