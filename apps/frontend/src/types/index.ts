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

export interface Supplier {
  id: number
  code: string
  name: string
  contact: string
  phone: string
  email: string
  address: string
  status: number
  remark: string
  created_at: string
  updated_at: string
}

export interface Customer {
  id: number
  code: string
  name: string
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
  supplier?: Supplier | null
  created_at: string
  updated_at: string
}
