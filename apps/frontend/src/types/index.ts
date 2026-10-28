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
