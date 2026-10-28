export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

export interface UserInfo {
  id: number
  username: string
  nickname: string
  role: string
}

export interface LoginResponse {
  token: string
  expires_at: number
  user: UserInfo
}

export interface DashboardSummary {
  product_total: number
  enabled_products: number
  supplier_total: number
  customer_total: number
}

/**
 * 主体角色，位掩码：1 客户 / 2 供应商 / 3 两者。
 *
 * 不是三选一的枚举。一个主体可以同时是客户和供应商（外贸里很常见），
 * 后端用位掩码存，所以判断角色要按位与，不能写 ===。
 */
export type PartnerRole = 1 | 2 | 3

/** 主体类型：0 公司 / 1 个人。个人主体的报关资料与发票抬头和公司不同。 */
export type PartnerEntityType = 0 | 1

/**
 * 商业伙伴（往来单位）。
 *
 * 客户与供应商是同一张表里的两种角色，不再是两个实体。
 * /customers 与 /suppliers 两个接口返回的也是这个结构，只是角色被路由锁定。
 */
export interface Partner {
  id: number
  code: string
  name: string
  role: PartnerRole
  entity_type: PartnerEntityType
  country: string
  contact: string
  phone: string
  email: string
  address: string
  status: number
  remark: string
  /** 商务条件：主要对客户角色有意义，供应商留空。 */
  trade_term: string
  settlement_method: string
  /** 账期天数，从出货或开票起算。0 表示款到发货。 */
  credit_days: number
  /** 信用额度，本币计。0 表示不限（不是「没有额度」）。 */
  credit_limit: number
  /** 默认结算币种，空表示按报价单或本币。 */
  currency_code: string
  created_at: string
  updated_at: string
}

/**
 * 往来单位下的具体自然人（CRM）。
 *
 * 一个主体可有多个联系人，各自有电话、邮箱、职位、语言。
 * md_partner.contact 只是主联系人姓名的冗余，不是这个实体。
 * is_primary 每个主体最多一个，由后端在事务里保证。
 */
export interface Contact {
  id: number
  partner_id: number
  name: string
  title: string
  phone: string
  email: string
  /** 决定谁去跟这个客户。 */
  language: string
  /** 偏好联系渠道：发错渠道等于没联系。 */
  channel: string
  is_primary: boolean
  status: number
  remark: string
  partner?: Partner | null
  created_at: string
  updated_at: string
}

/**
 * 跟进记录（CRM）。挂在主体上，可选指向某个联系人。
 *
 * occurred_at 与 created_at 是两件事：补录上周的电话时，
 * 录入时间是今天，但「最近一次跟进」是上周。
 */
export interface Activity {
  id: number
  partner_id: number
  contact_id: number | null
  /** CALL / EMAIL / VISIT / QUOTE / OTHER */
  type: string
  subject: string
  content: string
  occurred_at: string
  /** 为空表示不需要再跟；早于今天即算「已到期」。 */
  next_follow_at: string | null
  partner?: Partner | null
  contact?: Contact | null
  created_at: string
  updated_at: string
}

/**
 * 商机（CRM）。
 *
 * 阶段与单据状态是两套东西：商机阶段可以回退（客户预算冻结时真实地退回去了），
 * 赢单/输单是终态，要重启就新建一条。
 *
 * 金额四件套：amount（原币）+ amount_base（本币）+ exchange_rate + currency_code。
 * 商机是预计值，金额变更时按当日汇率重算；报价单与订单在那边锁定汇率。
 */
export interface Opportunity {
  id: number
  partner_id: number
  subject: string
  /** 10 接洽 / 20 需求确认 / 30 已报价 / 40 谈判中 / 50 赢单 / 60 输单 */
  stage: number
  amount: number
  currency_code: string
  amount_base: number
  exchange_rate: number
  /** 赢单恒 100、输单恒 0，由后端强制。 */
  probability: number
  expected_close_date: string
  remark: string
  partner?: Partner | null
  created_at: string
  updated_at: string
}

export interface OpportunityStageSummary {
  stage: number
  count: number
  amount_base: number
  weighted: number
}

export interface OpportunitySummary {
  stages: OpportunityStageSummary[]
  total_count: number
  total_amount_base: number
  total_weighted: number
}

/**
 * 线索（CRM）：尚未建档的询盘主体。
 *
 * 大多数询盘不会成交，全部建进往来单位会让客户列表变成通讯录。
 * status=30（已转化）是硬终态，此时 partner_id 指向新建的主体。
 */
export interface Lead {
  id: number
  company: string
  contact_name: string
  phone: string
  email: string
  country: string
  /** EXHIBITION / ALIBABA / GOOGLE / REFERRAL / OTHER */
  source: string
  demand: string
  /** 10 新建 / 20 跟进中 / 30 已转化 / 40 已废弃 */
  status: number
  partner_id: number | null
  converted_at: string | null
  remark: string
  partner?: Partner | null
  created_at: string
  updated_at: string
}

export interface LeadConvertResult {
  lead: Lead
  partner: Partner
  /** 同名同国主体的提示，非阻断。 */
  warning?: string
}

/**
 * 币种。三位 ISO 4217 代码是业务身份，单据上存的就是它，所以创建后不可修改。
 *
 * is_base 标记本币：全库有且只有一个，是折算的基准。
 */
export interface Currency {
  id: number
  code: string
  name: string
  symbol: string
  decimal_places: number
  is_base: boolean
  status: number
  remark: string
  created_at: string
  updated_at: string
}

/**
 * 汇率：1 单位 currency_code = rate 单位 base_code。
 *
 * 只追加不覆盖，没有编辑入口；effective_date 是业务日期（后端按 UTC 零点存），
 * 展示时只取日期部分，见 utils/format.ts 的 formatDate。
 */
export interface ExchangeRate {
  id: number
  currency_code: string
  base_code: string
  rate: number
  effective_date: string
  source: string
  remark: string
  created_at: string
  updated_at: string
}

export interface Product {
  id: number
  sku_code: string
  name_cn: string
  name_en: string
  category: string
  spec: string
  unit: string
  supplier_id: number | null
  cost_price: number
  sale_price: number
  status: number
  remark: string
  supplier?: Partner | null
  created_at: string
  updated_at: string
}

/**
 * 产品货源：某产品在某供应商处的采购来源与货号、价格、交期。
 *
 * 一个产品可挂多个货源，产品上的 supplier_id 只是默认货源。
 * 采购价按「原币 + 币种 + 本币 + 汇率」四件套存：只存原币的话，
 * 比价得手工乘汇率，而汇率每天在变，昨天的比价结论今天就不成立。
 *
 * 查重维度是「产品 + 供应商 + 货号」，不是全局货号：我方一个 SKU 拆成
 * 散装与彩盒两套编码、都从同一家同一个货号进货，是真实做法。
 */
export interface ProductSource {
  id: number
  product_id: number
  partner_id: number
  /** 供应商货号；允许为空——打样阶段对方还没给号。 */
  supplier_sku: string
  purchase_price: number
  currency_code: string
  purchase_price_base: number
  exchange_rate: number
  /** 最小起订量，0 表示对方没提要求。 */
  moq: number
  /** 交期天数，0 表示未确认。 */
  delivery_days: number
  status: number
  remark: string
  product?: Product | null
  partner?: Partner | null
  created_at: string
  updated_at: string
}

/**
 * 报价明细行。
 *
 * 金额、本币金额、成本价、成本额全部由后端算并锁在这里：成本是**快照**，
 * 供应商涨价后回看三个月前的报价，仍应显示当时的毛利。
 */
export interface QuotationItem {
  id: number
  quotation_id: number
  product_id: number
  /** 产品信息快照：产品改名后报价单上仍是发给客户的那个说法。 */
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: number
  /** 报价币种单价。 */
  unit_price: number
  amount: number
  amount_base: number
  /** 本币单位成本快照，来自货源。 */
  cost_price: number
  cost_amount: number
  /** 成本取自哪条货源，事后能回答「这个价按谁家算的」。 */
  source_id: number | null
  remark: string
  product?: Product | null
}

/**
 * 报价单。
 *
 * 与商机的区别在「承诺」：商机金额随汇率重算，报价单发出后金额、汇率、成本一律锁定。
 * 同一份报价谈多轮时共用 doc_no、只递增 version，所以「QT-202610-0001 的第 3 版」
 * 在邮件里能被唯一指认。
 */
export interface Quotation {
  id: number
  doc_no: string
  version: number
  /** 第 1 版的 id；第 1 版自身为 0。 */
  root_id: number
  partner_id: number
  trade_term: string
  /** 10 草稿 / 20 已确认 / 90 已取消 */
  status: number
  currency_code: string
  exchange_rate: number
  valid_until: string
  amount_total: number
  amount_base_total: number
  cost_total: number
  gross_profit: number
  /** 毛利率，0.235 表示 23.5%。 */
  margin_rate: number
  remark: string
  partner?: Partner | null
  items?: QuotationItem[]
  created_at: string
  updated_at: string
}

/** 详情比列表多两个由服务端算的字段：是否过期、为什么不能转订单。 */
export interface QuotationDetail extends Quotation {
  expired: boolean
  convert_block?: string
}

export interface QuotationPreviewItem {
  product_id: number
  sku_code: string
  name_cn: string
  unit: string
  qty: number
  unit_price: number
  amount: number
  amount_base: number
  cost_price: number
  cost_amount: number
  gross_profit: number
  source_id: number | null
  source_name: string
}

/** 报价试算结果：口径与保存后完全一致（同一个折算函数）。 */
export interface QuotationPreview {
  currency_code: string
  exchange_rate: number
  items: QuotationPreviewItem[]
  amount_total: number
  amount_base_total: number
  cost_total: number
  gross_profit: number
  margin_rate: number
}
