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

/**
 * 商务条件字典。取值必须与后端 model.TradeTerm / model.SettlementMethod 一致。
 *
 * 贸易术语不是标签而是报价的组成部分：FOB 的海运费买方承担、CIF 卖方承担，
 * 同一批货两个报价值差可以到 10%，所以界面上要能一眼看出报的是哪种。
 */
export const TRADE_TERM_OPTIONS = [
  { value: 'EXW', label: 'EXW 工厂交货' },
  { value: 'FOB', label: 'FOB 装运港交货' },
  { value: 'CFR', label: 'CFR 成本加运费' },
  { value: 'CIF', label: 'CIF 成本保险加运费' },
  { value: 'DDP', label: 'DDP 完税后交货' },
]

/**
 * 跟进方式。取值与后端 model.ActivityType 一致。
 *
 * 分类型不是为了好看：报价后又沉默三天，和报价后客户回了"价格太高"，
 * 是完全不同的处境，光看一条文字记录分不出来。
 */
export const ACTIVITY_TYPE_OPTIONS = [
  { value: 'CALL', label: '电话' },
  { value: 'EMAIL', label: '邮件' },
  { value: 'VISIT', label: '拜访' },
  { value: 'QUOTE', label: '报价' },
  { value: 'OTHER', label: '其它' },
]

/** 未知码原样透出，不吞数据。 */
export function activityTypeLabel(value?: string | null): string {
  if (!value) return '其它'
  return ACTIVITY_TYPE_OPTIONS.find((o) => o.value === value)?.label ?? value
}

/** 结算方式：决定账期与风险，也决定报价的价格基础。 */
export const SETTLEMENT_METHOD_OPTIONS = [
  { value: 'TT', label: 'T/T 电汇' },
  { value: 'LC', label: 'L/C 信用证' },
  { value: 'DP', label: 'D/P 付款交单' },
  { value: 'DA', label: 'D/A 承兑交单' },
  { value: 'OA', label: 'O/A 赊销' },
]
