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

/**
 * 运费杂费类型。取值与后端 model.ShipmentChargeType 一致，故意留空档：
 * 将来要在报关费与拖车费之间插一类（查验费），加一个值即可。
 */
export const SHIPMENT_CHARGE_OPTIONS = [
  { value: 10, label: '海运费' },
  { value: 20, label: '报关费' },
  { value: 30, label: '拖车费' },
  { value: 40, label: '保险费' },
  { value: 90, label: '其它' },
] as const

const SHIPMENT_CHARGE_MAP = new Map<number, string>(
  SHIPMENT_CHARGE_OPTIONS.map((o) => [o.value, o.label]),
)

/** 费用类型的中文名；未知值原样透出，不吞数据，便于发现后端加了新类型而前端没跟上。 */
export function shipmentChargeLabel(value?: number | null): string | undefined {
  if (value === null || value === undefined) return undefined
  return SHIPMENT_CHARGE_MAP.get(value) ?? `未知类型(${value})`
}

/**
 * 报关单状态。取值与后端 model.CustomsStatus 一致。
 *
 * 独立于单据状态机：报关单没有「备货中」「已出货」，
 * 而「已退单」是海关退回要求修改，改完还要重新申报，不是作废。
 */
export const CUSTOMS_STATUS_OPTIONS = [
  { value: 10, label: '待申报' },
  { value: 20, label: '已申报' },
  { value: 30, label: '已放行' },
  { value: 90, label: '已退单' },
] as const

const CUSTOMS_STATUS_MAP = new Map<number, string>(
  CUSTOMS_STATUS_OPTIONS.map((o) => [o.value, o.label]),
)

export function customsStatusLabel(value?: number | null): string | undefined {
  if (value === null || value === undefined) return undefined
  return CUSTOMS_STATUS_MAP.get(value) ?? `未知状态(${value})`
}

/** 「已退单」要用告警色，其余是进度色。 */
export function customsStatusColor(value?: number | null): string {
  if (value === 90) return 'red'
  if (value === 30) return 'green'
  if (value === 20) return 'blue'
  return 'default'
}

/**
 * 常用监管方式。做成可自由输入的候选而不是封闭枚举：
 * 海关的监管方式有几十种且会调整，封闭下拉会把人挡在门外。
 */
export const SUPERVISION_MODE_OPTIONS = [
  { value: '0110', label: '0110 一般贸易' },
  { value: '0615', label: '0615 进料对口' },
  { value: '1039', label: '1039 市场采购' },
  { value: '9610', label: '9610 跨境电商' },
  { value: '1210', label: '1210 保税电商' },
] as const

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
 * 主体角色位。1 客户 / 2 供应商 / 4 货代，叠加时按位相加
 * （3 = 客供两用，5 = 客户兼货代）。
 *
 * 这里是「位」而不是「选项」：角色可以叠加，所以没有值为 3 的第三个选项，
 * 界面上的多选由位运算合成，展示由 RoleTag 拆开。取值必须与后端
 * model.PartnerRole 一致，新增角色只在这里与那里各补一行。
 */
export const PARTNER_ROLE = {
  customer: 1,
  supplier: 2,
  forwarder: 4,
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

/**
 * 商机阶段。取值必须与后端 model.OpportunityStage 一致。
 *
 * probability 是默认赢单率，不是固定值：同一个"已报价"，老客户 70%、新客户 30%，
 * 按阶段反推概率会把这两种情况算成一个数。
 */
export const OPPORTUNITY_STAGE_OPTIONS = [
  { value: 10, label: '初步接洽', probability: 10 },
  { value: 20, label: '需求确认', probability: 30 },
  { value: 30, label: '已报价', probability: 50 },
  { value: 40, label: '谈判中', probability: 70 },
  { value: 50, label: '赢单', probability: 100 },
  { value: 60, label: '输单', probability: 0 },
]

export function opportunityStageLabel(value?: number | null): string {
  if (value === null || value === undefined) return '未知阶段'
  return OPPORTUNITY_STAGE_OPTIONS.find((o) => o.value === value)?.label ?? `阶段 ${value}`
}

/** 终态之外的阶段都算"在谈"。 */
export function isOpenStage(value?: number | null): boolean {
  return value !== 50 && value !== 60
}

/** 线索状态。取值与后端 model.LeadStatus 一致；30 是硬终态。 */
export const LEAD_STATUS_OPTIONS = [
  { value: 10, label: '新建' },
  { value: 20, label: '跟进中' },
  { value: 30, label: '已转化' },
  { value: 40, label: '已废弃' },
]

export const LEAD_SOURCE_OPTIONS = [
  { value: 'EXHIBITION', label: '展会' },
  { value: 'ALIBABA', label: '阿里国际站' },
  { value: 'GOOGLE', label: '搜索引擎' },
  { value: 'REFERRAL', label: '老客户推荐' },
  { value: 'OTHER', label: '其它' },
]

export function leadStatusLabel(value?: number | null): string {
  if (value === null || value === undefined) return '未知'
  return LEAD_STATUS_OPTIONS.find((o) => o.value === value)?.label ?? `状态 ${value}`
}

export function leadSourceLabel(value?: string | null): string {
  if (!value) return '—'
  return LEAD_SOURCE_OPTIONS.find((o) => o.value === value)?.label ?? value
}

/**
 * 报价单状态。取值与后端 model.DocumentStatus 一致，但只列报价单真会走到的三个：
 * 备货中 / 已出货属于订单，报价单永远不该出现（后端也会拒绝）。
 */
export const QUOTATION_STATUS_OPTIONS = [
  { value: 10, label: '草稿' },
  { value: 20, label: '已确认' },
  { value: 90, label: '已取消' },
]

export function quotationStatusLabel(value?: number | null): string {
  if (value === null || value === undefined) return '未知'
  return QUOTATION_STATUS_OPTIONS.find((o) => o.value === value)?.label ?? `状态 ${value}`
}

/**
 * 销售订单状态：完整的单据状态机。
 * 跳步（草稿直接到已出货）与往回退都由后端拒绝，前端只在详情页给出当前可走的下一步。
 */
export const ORDER_STATUS_OPTIONS = [
  { value: 10, label: '草稿' },
  { value: 20, label: '已确认' },
  { value: 30, label: '备货中' },
  { value: 40, label: '已出货' },
  { value: 50, label: '已完成' },
  { value: 90, label: '已取消' },
]

export function orderStatusLabel(value?: number | null): string {
  if (value === null || value === undefined) return '未知'
  return ORDER_STATUS_OPTIONS.find((o) => o.value === value)?.label ?? `状态 ${value}`
}

/**
 * 库存流水类型。取值与后端 `model.StockMovementType` 一致。
 *
 * 类型只说明"为什么动"，不决定方向：调拨有调入调出，盘点有盘盈盘亏。
 */
export const STOCK_MOVEMENT_OPTIONS = [
  { value: 10, label: '期初' },
  { value: 20, label: '采购入库' },
  { value: 30, label: '销售出库' },
  { value: 40, label: '调拨' },
  { value: 50, label: '盘点盈亏' },
  { value: 90, label: '其它' },
]

/**
 * 可手工录入的类型。
 *
 * 采购入库与销售出库不在其中：它们必须挂在入库单 / 出库单上（T-16），
 * 后端会拒（400）。给出来只会让人先选再被拒。
 */
export const MANUAL_STOCK_MOVEMENT_OPTIONS = [
  { value: 10, label: '期初' },
  { value: 50, label: '盘点盈亏' },
  { value: 90, label: '其它' },
]

export function stockMovementLabel(value?: number | null): string {
  if (value === null || value === undefined) return '未知'
  return STOCK_MOVEMENT_OPTIONS.find((o) => o.value === value)?.label ?? `类型 ${value}`
}

/** 出入方向：1 入库 / -1 出库。 */
export const STOCK_DIRECTION_OPTIONS = [
  { value: 1, label: '入库' },
  { value: -1, label: '出库' },
]

/**
 * 出入库单类型。与后端 `model.StockDocType` 一致。
 *
 * 调拨（30）不在这里：它需要仓库维度才能成立，后端会拒（400）。
 */
export const STOCK_DOC_TYPE_OPTIONS = [
  { value: 10, label: '采购入库' },
  { value: 20, label: '销售出库' },
  { value: 40, label: '盘点' },
]

export function stockDocTypeLabel(value?: number | null): string {
  if (value === null || value === undefined) return '未知'
  return STOCK_DOC_TYPE_OPTIONS.find((o) => o.value === value)?.label ?? `类型 ${value}`
}

/** 盘点单：明细填的是实盘数，账面数在确认时取快照。 */
export const STOCK_DOC_COUNT = 40

/** 贸易术语的中文名；未知码原样透出，不吞数据。 */
export function tradeTermLabel(value?: string | null): string {
  if (!value) return '—'
  return TRADE_TERM_OPTIONS.find((o) => o.value === value)?.label ?? value
}

/** 结算方式：决定账期与风险，也决定报价的价格基础。 */
export const SETTLEMENT_METHOD_OPTIONS = [
  { value: 'TT', label: 'T/T 电汇' },
  { value: 'LC', label: 'L/C 信用证' },
  { value: 'DP', label: 'D/P 付款交单' },
  { value: 'DA', label: 'D/A 承兑交单' },
  { value: 'OA', label: 'O/A 赊销' },
]
