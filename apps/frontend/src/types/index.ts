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
