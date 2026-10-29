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

/**
 * 系统账号（用户维护页用，T-57）。
 *
 * 比 UserInfo 多一个 status：登录响应里只带启用账号，维护页要看到停用的。
 * 密码永远不会出现在任何响应里，后端字段标了 json:"-"。
 */
export interface SystemUser {
  id: number
  username: string
  nickname: string
  role: string
  status: number
  created_at: string
  updated_at: string
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
 * 地址用途。与后端 model.PartnerAddressType 一致。
 *
 * 四种而不是"多地址一锅端"：一张单证上同时出现发票抬头、收货人、通知方，
 * 三者常常分属不同主体，取址必须按用途区分。
 */
export type PartnerAddressType = 'invoice' | 'delivery' | 'consignee' | 'notify'

/** 主体的一个地址。每个（主体，用途）最多一条 `is_default`。 */
export interface PartnerAddress {
  id: number
  partner_id: number
  type: PartnerAddressType
  is_default: boolean
  contact: string
  phone: string
  country: string
  province: string
  city: string
  postal_code: string
  /** 门牌与街道，不含国家省市——那三项单独存，单证按行取。 */
  address: string
  remark: string
  created_at: string
  updated_at: string
}

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
  /** 已软删时才有值（Unix 毫秒），未删除时后端不返回这个字段。 */
  deleted_at?: number
  country: string
  contact: string
  phone: string
  email: string
  address: string
  /** 纳税人识别号：开票与报关用。不强制，但单证上缺它会被客户或海关退回。 */
  tax_id: string
  /** 报表分组用的客户分组（如「南美批发商」），自由文本。 */
  customer_group: string
  /** 我方划分的销售片区，与 country 不是一回事：一个国家可以有多个片区。 */
  region: string
  /** ABC 分级。空串表示未分级；**只做标签与筛选，不接权限也不接折扣**。 */
  grade: 'A' | 'B' | 'C' | ''
  status: number
  remark: string
  /** 商务条件：主要对客户角色有意义，供应商留空。 */
  trade_term: string
  settlement_method: string
  /** 账期天数，从出货或开票起算。0 表示款到发货。 */
  credit_days: number
  /** 信用额度，本币计。0 表示不限（不是「没有额度」）。定点字符串。 */
  credit_limit: string
  /** 默认结算币种，空表示按报价单或本币。 */
  currency_code: string
  /**
   * 负责人（sys_user.id）。0 表示**未分配**（公共池）：刚上线的存量数据与
   * "先录进来还没分人"的客户都在这一堆，它们对任何业务员都可见。
   */
  owner_id: number
  /** 负责人登录名，后端列表 join 出来的只读投影。 */
  owner_name?: string
  created_at: string
  updated_at: string
}

/**
 * 一条查重候选：某个已有主体与正在录入的记录像在哪里。
 *
 * 只提示不阻断——外贸同名公司在不同国家确实存在，拦住等于让人把询盘丢掉。
 * reasons 是命中的规则说明（名称+国家 / 邮箱同域 / 电话尾号），可能不止一条。
 */
export interface PartnerDuplicateCandidate {
  partner: Partner
  reasons: string[]
}

/** 导入失败的一行：行号让人能回到文件里那一行去改。 */
export interface PartnerImportFailure {
  line: number
  code: string
  name: string
  reason: string
}

/** 导入时的可疑重复：只提示不阻断（外贸同名公司跨国家确实存在）。 */
export interface PartnerImportDuplicate {
  line: number
  code: string
  name: string
  reasons: string[]
}

/**
 * 主体导入报告（T-35）。
 *
 * `dry_run=true` 时只校验不落库，`created` 一定是 0——先看报告再决定导不导。
 */
export interface PartnerImportReport {
  dry_run: boolean
  total: number
  created: number
  skipped: number
  failures: PartnerImportFailure[]
  duplicates: PartnerImportDuplicate[]
}

/**
 * 合并主体的结果。
 *
 * partner 是留下的那条（角色已并集、空字符串字段已补齐），absorbed 是被并入并软删的
 * 那条，moved 是各表被迁移的引用条数——合并不可逆，至少要能说清动过哪些表。
 */
export interface PartnerMergeResult {
  partner: Partner
  absorbed: Partner
  moved: Record<string, number>
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
  /** 后端 money.Decimal 序列化成字符串，前端只做展示与转交，不做算术。 */
  amount: string
  currency_code: string
  amount_base: string
  exchange_rate: string
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
  amount_base: string
  weighted: string
}

export interface OpportunitySummary {
  stages: OpportunityStageSummary[]
  total_count: number
  total_amount_base: string
  total_weighted: string
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
  /** 定点字符串（后端 money.Decimal）：折算本身在服务端做，前端只展示。 */
  rate: string
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
  /** 后端 money.Decimal：字符串收发，前端只显示，不参与算术。 */
  cost_price: string
  /**
   * 成本价被**服务端**清过（T-36 字段级权限）。
   *
   * 没有这个标记，被清成 0 的成本价与"这件货不要钱"在界面上分不开——
   * 两者含义正好相反，所以界面要显示「—」而不是 0。
   */
  cost_hidden?: boolean
  sale_price: string
  /**
   * 包装规格：每箱数量、单箱毛净重（kg）、单箱体积（m³）、外箱尺寸。
   *
   * 全部可选，0 表示未维护。维护之后出货分配行的箱数与毛净重由
   * 「箱数 × 单箱值」推导，订舱要的 CBM 也随之算出来，不再人工算。
   */
  pack_qty: number
  carton_gross_weight: string
  carton_net_weight: string
  carton_volume: string
  carton_size: string
  /** 报关要素的默认值。真实报关以出货批次上填的为准，这里是带出来的缺省。 */
  hs_code: string
  declared_name_cn: string
  declared_name_en: string
  material: string
  origin_country: string
  legal_unit: string
  status: number
  remark: string
  supplier?: Partner | null
  /**
   * 这次搜索命中的编号（旧 SKU / 客户号…）。只在带 keyword 的列表里出现，
   * 详情接口不返回：它是某一次查询的结果，不是产品的一个字段。
   */
  matched_codes?: string[]
  /**
   * 已软删时才有值（Unix 毫秒）。后端用 omitempty 序列化，
   * 未删除的行不会出现这个字段——「含已删除」的列表靠它标记行与显示「恢复」。
   */
  deleted_at?: number
  created_at: string
  updated_at: string
}

/**
 * 运费杂费：归集到一票货的成本，单票毛利的减项。
 *
 * 只登记我方承担的费用 —— FOB 下客户付的海运费不录，录进来会被算成我方成本。
 * 金额按「原币 + 币种 + 本币 + 汇率」四件套存：只存原币的话，
 * 把美元运费和人民币报关费加在一起就得手工换算，而汇率每天都在变。
 */
export interface ShipmentCharge {
  id: number
  shipment_id: number
  /** 10 海运费 / 20 报关费 / 30 拖车费 / 40 保险费 / 90 其它 */
  type: number
  /** 收款方（货代或供应商），可为空。不表示"谁承担"，这张表里每行都是我方的成本。 */
  partner_id: number | null
  /** 金额与汇率是定点字符串：前端只展示，算术在服务端。 */
  amount: string
  currency_code: string
  amount_base: string
  exchange_rate: string
  remark: string
  partner?: Partner | null
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
  purchase_price: string
  currency_code: string
  purchase_price_base: string
  exchange_rate: string
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
  /** 数量与金额一样是后端 money.Decimal 的定点字符串：3 位小数，前端不参与算术。 */
  qty: string
  /** 报价币种单价。金额字段都是后端 money.Decimal 的字符串，前端只展示不运算。 */
  unit_price: string
  amount: string
  amount_base: string
  /** 本币单位成本快照，来自货源。 */
  cost_price: string
  cost_amount: string
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
  /** 案号（DZ2603）：报价与它转出的订单共用，一笔生意一个。 */
  base_no: string
  version: number
  /** 第 1 版的 id；第 1 版自身为 0。 */
  root_id: number
  partner_id: number
  trade_term: string
  /** 10 草稿 / 20 已确认 / 90 已取消 */
  status: number
  currency_code: string
  exchange_rate: string
  valid_until: string
  amount_total: string
  amount_base_total: string
  cost_total: string
  gross_profit: string
  /** 毛利率，0.235 表示 23.5%。字符串，展示时再转。 */
  margin_rate: string
  remark: string
  /** 转订单后回填：一张报价单最多转成一张订单。 */
  sales_order_id?: number | null
  sales_order_doc_no?: string
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

/**
 * 合同的一条付款分期：30% 订金 / 70% 见提单副本。
 *
 * 本币金额随行存一份：分期报表要按本币汇总，每次再乘一遍汇率既慢，
 * 又会和合同总额差一个末位（舍入位置不同）。
 */
export interface ContractTerm {
  id: number
  contract_id: number
  /** 展示顺序，从 1 开始：付款有先后，顺序不是装饰。 */
  seq: number
  name: string
  /** 原币金额（合同币种）。 */
  amount: number
  /** 本币金额，按合同汇率折算。 */
  amount_base: number
  /** 约定付款日，可空。 */
  due_date: string | null
  /** "见提单副本后 7 天内"这类条件，与付款日可同时存在。 */
  trigger: string
}

/**
 * 合同：形式发票（PI）与销售合同共用一张表。
 *
 * **没有货物明细**：明细留在报价单或订单上，这里只有一份对外承诺的快照。
 * 金额与汇率在生成时照抄来源单据，之后不重取当日汇率——转单那一刻改价
 * 等于单方面改掉已经报出去的价。
 */
export interface Contract {
  id: number
  /** PI- 前缀是形式发票，SC- 是销售合同。 */
  doc_no: string
  /** 10 形式发票 / 20 销售合同 */
  type: number
  partner_id: number
  /** 10 报价单 / 20 销售订单 */
  source_type: number
  source_id: number
  source_doc_no: string
  /** 10 草稿 / 20 已确认 / 90 已取消 */
  status: number
  sign_date: string
  delivery_date: string | null
  currency_code: string
  /** 金额与汇率是定点字符串：前端只展示，算术在服务端。 */
  exchange_rate: string
  amount_total: string
  amount_base_total: string
  remark: string
  partner?: Partner | null
  /** 列表不带分期，只有详情带。 */
  terms?: ContractTerm[]
  /** 条款**快照**：生成时从模板抄下来的，改模板不会影响这里。 */
  clauses?: ContractClause[]
  created_at: string
  updated_at: string
}

/** 合同上的一条条款快照（T-27）。正文是当时的抄件，不是对模板的引用。 */
export interface ContractClause {
  id: number
  contract_id: number
  /** 来源模板 id，可空（手写条款没有模板）。 */
  clause_id: number | null
  seq: number
  title: string
  body: string
}

/**
 * 可复用的合同条款模板（主数据）。
 *
 * 注意别与 `ContractTerm` 混了：那个是**付款分期**（30% 订金 / 70% 见提单副本），
 * 这个是文本条款。
 */
export interface Clause {
  id: number
  code: string
  title: string
  body: string
  /** 付款 / 质量 / 违约 …… 自由文本，不做枚举。 */
  category: string
  status: number
  remark: string
  /** 已软删时才有值（Unix 毫秒），未删时后端不返回这个字段。 */
  deleted_at?: number
  created_at: string
  updated_at: string
}

/**
 * 销售订单明细。
 *
 * 与报价明细的区别：这里没有成本价与成本额。订单是"客户认下的量"，
 * 成本属于采购（T-12），毛利要等采购价与运杂费落定才算。
 */
export interface SalesOrderItem {
  id: number
  sales_order_id: number
  product_id: number
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: string
  /** 金额字段是后端 money.Decimal 的字符串：前端只展示，算术一律在服务端。 */
  unit_price: string
  amount: string
  amount_base: string
  remark: string
  product?: Product | null
  /** 已出数量，只在订单详情里由出货分配算出来；列表不带。 */
  shipped_qty?: string
  /** 未出数量 = qty - shipped_qty；列表不带。 */
  unshipped_qty?: string
}

/**
 * 销售订单。
 *
 * 合计只由明细汇总（后端算，前端提交的总额被忽略）。
 * 状态走完整单据状态机：10 草稿 → 20 已确认 → 30 备货中 → 40 已出货 → 50 已完成，
 * 未出货前可 90 取消；跳步会被拒绝。
 */
export interface SalesOrder {
  id: number
  doc_no: string
  /** 案号（DZ2603）：从报价转来时沿用来源的案号，手建则取新的一笔。 */
  base_no: string
  partner_id: number
  trade_term: string
  status: number
  order_date: string
  /** 约定交期，可空——下单时还没谈定是常态。 */
  delivery_date?: string | null
  currency_code: string
  exchange_rate: string
  amount_total: string
  amount_base_total: string
  remark: string
  partner?: Partner | null
  items?: SalesOrderItem[]
  created_at: string
  updated_at: string
}

/**
 * 变更单里的一行：某条订单明细的前后值。
 *
 * 只有真正变了的行才有记录——把没动的行也抄一遍，等于每次变更复制整张订单，
 * 而"哪几行变了"才是这张表要回答的问题。
 */
export interface SalesOrderChangeLine {
  id: number
  change_id: number
  sales_order_item_id: number
  /** 变更当时的 SKU 快照：订单行将来不在了，这条记录仍要能读懂。 */
  sku_code: string
  before_qty: string
  after_qty: string
  before_unit_price: string
  after_unit_price: string
}

/**
 * 已确认订单的一次变更。
 *
 * 订单本身更新到新值，这里只留痕：谁、什么时候、因为什么、把什么改成了什么。
 * 单号由订单号派生（`{订单号}-CH{n}`），脱离订单没有意义，所以不单独发号。
 */
export interface SalesOrderChange {
  id: number
  sales_order_id: number
  /** 同一张订单的第几次变更，从 1 开始。 */
  seq: number
  doc_no: string
  reason: string
  /** 操作人用户名的快照，不是外键：账号改名后历史仍要读出当时是谁。 */
  operator: string
  before_total: string
  after_total: string
  before_delivery_date?: string | null
  after_delivery_date?: string | null
  lines?: SalesOrderChangeLine[]
  created_at: string
  updated_at: string
}

/**
 * 出货分配行：这票货里有哪些销售订单行、各多少。
 *
 * 挂的是订单**行**而不是订单头：一张订单里两行可能是同一产品的不同包装，
 * 按订单头分配就分不清出的到底是哪一种。
 * sku_code / name_cn / spec / unit 是出货当时的产品快照。
 */
export interface ShipmentAllocation {
  id: number
  shipment_id: number
  sales_order_item_id: number
  /** 来源订单号，后端摊平后给出，省得前端顺着三层对象去翻。 */
  sales_order_doc_no: string
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: string
  /**
   * 装箱数据，只有印装箱单时才需要；0 与空串表示"还没装箱"。
   *
   * 重量是**这一行的合计**，不是单箱：合计直接相加就能得到整票的 G.W. / N.W.，
   * 按单箱记的话改一次箱数就要把所有合计重算一遍。
   */
  cartons: number
  gross_weight: string
  net_weight: string
  /** 外箱尺寸，如 "60×40×30CM"，系统只原样印出来，不算体积。 */
  measurement: string
  remark: string
}

/**
 * 出货批次：一次实际发运。
 *
 * 与订单是多对多——一张订单可以分几票走，一票也能装几张订单的货（拼柜）。
 * 没有金额字段：出货本身不收钱也不付钱，费用归集在出货杂费（T-18）。
 */
export interface Shipment {
  id: number
  /** 送货单号，同时也是这一票的套号：商业发票与装箱单共用它。 */
  doc_no: string
  status: number
  /** 商票改证次数：0 是首版，改证不换号，只在文件名与打印件上加 -R1。 */
  invoice_revision: number
  /** 实际（或计划）出货日，决定它落在哪个统计月份。 */
  ship_date: string
  /** 运杂费摊到行的依据（T-59）：amount / qty / volume / weight / cartons。 */
  charge_alloc_basis: string
  /**
   * 物流节点。它们是订舱之后才知道的，所以不走编辑页：
   * 用 `shipmentApi.updateLogistics` 单独保存，任何状态都能改。
   */
  bl_no: string
  vessel: string
  /** YYYY-MM-DD，可能为 null（未定）。 */
  etd: string | null
  eta: string | null
  remark: string
  allocations?: ShipmentAllocation[]
  created_at: string
  updated_at: string
}

/**
 * 应收 / 应付共有结构。
 *
 * 后端两张表共用一组列（`model.Settlement`），前端也共用一个类型：
 * 差别只有"对方是客户还是供应商"与"来源是哪张单据"，那两件事由字段值说明。
 *
 * 汇率是**开单时锁定**的，不随汇率表变动——否则同一张单在两天里会显示
 * 两个本币金额，对账时无法解释。
 */
export interface Settlement {
  id: number
  doc_no: string
  partner_id: number
  /** 10 销售订单 / 20 出货批次 / 30 采购订单 */
  source_type: number
  source_id: number
  /** 来源单号，摊平存一份，列表页不必再 join。 */
  source_doc_no: string
  currency_code: string
  /** 金额与汇率是定点字符串：前端只展示，算术在服务端。 */
  exchange_rate: string
  amount: string
  amount_base: string
  /** 已核销金额，派生值：唯一维护方是核销（T-21）。 */
  settled_amount: string
  settled_amount_base: string
  /** YYYY-MM-DD。账龄分桶的唯一依据，不可为空。 */
  due_date: string
  /** 10 未结清 / 20 部分结清 / 30 已结清 / 90 已作废 */
  status: number
  remark: string
  partner?: Partner | null
  created_at: string
  updated_at: string
}

/** 应收单与应付单结构相同，分开取名只为让调用处读得懂。 */
export type Receivable = Settlement
export type Payable = Settlement

/**
 * 收付款核销行：这笔钱还的是哪张单、还多少。
 *
 * 只增不改：录错了删掉整张收付款单，目标的已核销金额随单一起回滚——
 * 核销不是冲销，不在这一行上改负数。
 */
export interface PaymentAllocation {
  id: number
  payment_id: number
  /** 10 应收单 / 20 应付单 */
  target_type: number
  target_id: number
  /** 目标单号，摊平存一份，清单不必再 join 应收应付表。 */
  target_doc_no: string
  /** 核销的原币金额，币种跟随收付款单。定点字符串。 */
  amount: string
  /** 本币金额，按**收付款单的汇率**折——与目标单据锁定本币之差就是汇兑损益。 */
  amount_base: string
  created_at: string
  updated_at: string
}

/**
 * 收付款单：实际收到 / 付出的一笔钱。
 *
 * 汇率取**到账日**，不是应收单上锁的开单日：同一个 1000 美元，开票 7.1、到账 7.3，
 * 差额才是汇兑损益。已核销金额 = 核销行之和，小于金额的部分是预收 / 预付。
 */
/**
 * 账龄分桶：按逾期天数把未核销金额切开。
 *
 * 桶边界只有后端一份（未到期 / 1-30 / 31-60 / 61-90 / 90+），前端只负责显示。
 * 在前端重算一遍，同一批数据就会有两个账龄，而客户只认其中一个。
 */
export interface AgingBuckets {
  /** 定点字符串（后端 money.Decimal）：分桶的算术在服务端，前端只显示。 */
  not_due: string
  days_1_30: string
  days_31_60: string
  days_61_90: string
  days_over_90: string
}

/** 一行账龄：一个主体的一种币种。跨币种的原币金额不可相加，所以按币种拆行。 */
export interface AgingRow {
  partner_id: number
  partner_code: string
  partner_name: string
  currency_code: string
  /** 未核销的原币分桶。 */
  amount: AgingBuckets
  /** 同样口径的本币分桶（按单据上锁定的汇率折）。 */
  amount_base: AgingBuckets
  total: string
  total_base: string
  /** YYYY-MM-DD：这一行里最早到期的那张单，催款从这里开始。 */
  earliest_due_date: string
}

/**
 * 一张已逾期且尚未结清的单据（T-29 的催款清单）。
 *
 * 逐笔而不是按客户聚合：客户欠三张单时，只说"他欠 8 万"没法决定先催哪一张。
 */
export interface CollectionRow {
  id: number
  doc_no: string
  partner_id: number
  partner_code: string
  partner_name: string
  currency_code: string
  /** YYYY-MM-DD */
  due_date: string
  overdue_days: number
  /** 未核销的原币金额与本币金额。 */
  outstanding: string
  outstanding_base: string
}

/**
 * 回款与超期报表。
 *
 * 一张表里两个数，**不能互相校验**：回款率是流量（分子按到账日、分母按到期日），
 * 超期明细是截至 as_of 的存量。收的钱可能是在还上个月的账，所以回款率超过 100% 正常。
 */
export interface CollectionReport {
  /** 10 应收 / 20 应付 */
  direction: number
  from: string
  to: string
  as_of: string
  /** 期间内到期的单据金额（本币），回款率的分母。 */
  due_amount_base: string
  /** 期间内到账并核销的金额（本币），回款率的分子。 */
  received_base: string
  /** 0.6 = 60%。分母为 0 时给 0。 */
  collection_rate: string
  /** 截至 as_of 仍未核销的总额（含未到期）。 */
  outstanding_base: string
  /** 其中已逾期的那部分，以及它的账龄分桶（not_due 恒为 0）。 */
  overdue_base: string
  overdue_buckets: AgingBuckets
  overdue_rows: CollectionRow[]
  /** 明细被 limit 截断；overdue_base 与分桶仍是全量。 */
  truncated: boolean
}

/** 某个产品在**一票**货里的获利情况（T-31 下钻）。按票聚合，不是按订单行。 */
export interface ProductProfitRow {
  shipment_id: number
  doc_no: string
  ship_date: string
  partner_id: number
  partner_name: string
  qty: string
  revenue: string
  cost: string
  charge: string
  profit: string
  cost_missing: boolean
  /** 成本 / 运杂费 / 毛利被服务端清掉（T-36）：收入照给，赚多少看不到。 */
  cost_hidden?: boolean
}

export interface ProductProfitReport {
  product_id: number
  from: string
  to: string
  rows: ProductProfitRow[]
  revenue: string
  cost: string
  charge: string
  profit: string
  /** 0.2254 = 22.54%。收入为 0 时给 0。 */
  margin: string
  cost_missing: boolean
  /** 成本 / 运杂费 / 毛利 / 毛利率被服务端清掉（T-36）。 */
  cost_hidden?: boolean
}

/** 一条该跟进的记录（T-34）。到期当天不算，所以 overdue_days 从 1 起。 */
export interface TodoFollowup {
  id: number
  partner_id: number
  partner_name: string
  subject: string
  next_follow_at: string
  overdue_days: number
}

/** 一张该催的应收。到期当天就算，所以 overdue_days 可能是 0。 */
export interface TodoReceivable {
  id: number
  doc_no: string
  partner_id: number
  partner_name: string
  due_date: string
  outstanding_base: string
  overdue_days: number
}

/** 一个今天没有汇率的外币。 */
export interface TodoCurrency {
  code: string
  name: string
}

/**
 * 待办清单。
 *
 * 每类都给「全量条数 + 前 N 条」：条数回答"还剩多少事没做完"，
 * 清单回答"先做哪几件"。
 */
export interface TodoReport {
  as_of: string
  followups: TodoFollowup[]
  followup_count: number
  receivables: TodoReceivable[]
  receivable_count: number
  missing_rates: TodoCurrency[]
}

/** 客户 360 的抬头数字，全部本币。 */
export interface Partner360Totals {
  order_count: number
  order_amount_base: string
  last_order_date: string
  /** 含该客户订单行的出货票数（去重）：拼柜时一票里还有别家的货。 */
  shipment_count: number
  activity_count: number
  last_activity_at: string
  /** 只算未终结（赢单 / 输单之外）的商机。 */
  open_opportunity_count: number
  open_opportunity_amount_base: string
  receivable_outstanding_base: string
  receivable_overdue_base: string
}

export interface Partner360Activity {
  id: number
  type: string
  subject: string
  occurred_at: string
  next_follow_at: string
}

export interface Partner360Opportunity {
  id: number
  subject: string
  stage: number
  amount_base: string
  probability: number
  expected_close_date: string
}

export interface Partner360Order {
  id: number
  doc_no: string
  order_date: string
  status: number
  currency_code: string
  amount_base_total: string
}

export interface Partner360Shipment {
  id: number
  doc_no: string
  ship_date: string
  status: number
}

export interface Partner360Receivable {
  id: number
  doc_no: string
  currency_code: string
  due_date: string
  outstanding: string
  outstanding_base: string
  overdue_days: number
}

/**
 * 客户 360：一个主体的全貌。
 *
 * 每块默认 10 条且都带 id / 单号，能点进源单据；`totals` 永远是全量，
 * 不受每块截断影响。
 */
export interface Partner360 {
  partner_id: number
  partner_code: string
  partner_name: string
  country: string
  /** 角色位掩码：同一家可能既是客户又是供应商。 */
  role: number
  as_of: string
  totals: Partner360Totals
  activities: Partner360Activity[]
  opportunities: Partner360Opportunity[]
  orders: Partner360Order[]
  shipments: Partner360Shipment[]
  receivables: Partner360Receivable[]
}

/** 账龄报表。合计只给本币：把美元与人民币的原币金额加在一起没有意义。 */
export interface AgingReport {
  /** 10 应收 / 20 应付 */
  direction: number
  /** YYYY-MM-DD：分桶的截止日。 */
  as_of: string
  rows: AgingRow[]
  total_base: AgingBuckets
  grand_total_base: string
}

/**
 * 一笔核销产生的汇兑损益。
 *
 * 到账汇率与目标单据锁定的汇率不同，同一笔原币折出来的本币就不同：
 * 应收差额记收益（正），应付差额记损失（负）。预收 / 预付款没有核销对象，
 * 不出现在这里——还没核销就没有可比的单据汇率。
 */
export interface ExchangeGainLine {
  payment_id: number
  payment_doc_no: string
  pay_date: string
  /** 10 收款 / 20 付款 */
  direction: number
  partner_id: number
  partner_code: string
  partner_name: string
  currency_code: string
  /** 10 应收单 / 20 应付单 */
  target_type: number
  target_doc_no: string
  /** 核销原币。定点字符串（后端 money.Decimal）。 */
  amount: string
  /** 按到账汇率折的本币 */
  amount_base: string
  payment_rate: string
  document_rate: string
  /** 按单据汇率折出来的同一笔原币 */
  document_base: string
  /** 正数收益、负数损失，单位本币 */
  gain_base: string
}

export interface ExchangeGainSubtotal {
  currency_code: string
  amount: string
  gain_base: string
}

export interface ExchangeGainReport {
  from: string
  to: string
  lines: ExchangeGainLine[]
  subtotals: ExchangeGainSubtotal[]
  total_gain_base: string
}

/**
 * 销售看板的一行（T-28）：同一维度键下的金额汇总，全部本币。
 *
 * `order_amount` 与 `revenue` **不是同一批单据**：前者按下单日统计的订单金额，
 * 后者是出货日在范围内那部分货的收入。所以两者相减没有意义，表头也要分开标注。
 */
export interface SalesDashboardRow {
  key: string
  label: string
  order_amount: string
  revenue: string
  cost: string
  charge: string
  profit: string
  /** 毛利率：0.2254 = 22.54%。收入为 0 时给 0。 */
  margin: string
  /** 这一行涉及的出货票数（去重）。 */
  shipments: number
  /** 有出货行没有可用采购成本：此时成本与毛利都不完整。 */
  cost_missing: boolean
  /** 成本 / 运杂费 / 毛利 / 毛利率被服务端清掉（T-36）：订单额与收入照给。 */
  cost_hidden?: boolean
}

export interface SalesDashboard {
  dimension: 'customer' | 'product' | 'country' | 'month'
  from: string
  to: string
  rows: SalesDashboardRow[]
  /** 全量合计，不受 `limit` 截断影响。 */
  total: SalesDashboardRow
  truncated: boolean
}

/**
 * 一条操作日志（T-33）。
 *
 * 记的是**提交意图**：谁在什么时候对哪张单提交了什么、结果如何，
 * 不是数据库的前后 diff。`body` 已脱敏并截断。
 */
export interface AuditLog {
  id: number
  actor_id: number
  actor_name: string
  actor_role: string
  /** HTTP 方法。 */
  method: string
  /** 业务动作：create / update / delete / status / merge / restore … */
  action: string
  /** 资源名，如 sales-orders。 */
  entity: string
  /** 路径上的 :id；集合级操作（如重算快照）为 0。 */
  entity_id: number
  path: string
  route: string
  status: number
  request_id: string
  ip: string
  body: string
  created_at: string
}

/**
 * 客户当前占用的信用额度，单位本币。
 *
 * 敞口 = 未结清应收 + 已确认未开票订单。只算前一半会漏掉已经承诺出去、还没开票的货。
 */
export interface CreditSummary {
  partner_id: number
  /** 0 表示不限（不是"没有额度"）。定点字符串。 */
  credit_limit: string
  unlimited: boolean
  open_receivable: string
  unbilled_order: string
  exposure: string
  /** 额度减敞口；超了就是负数。不限额度时为 0。定点字符串。 */
  available: string
}

/**
 * 单票毛利。金额全是本币：外币收入与人民币成本只有折到同一个币种才能相减。
 *
 * 口径：毛利 = 收入 − 采购成本 − 运杂费。汇兑损益不在这里（T-23 单独成行）。
 */
export interface ShipmentProfit {
  shipment_id: number
  doc_no: string
  ship_date: string
  status: number
  /** 金额是定点字符串：毛利算在服务端，前端只展示。 */
  revenue: string
  cost: string
  charge: string
  profit: string
  /** 毛利率，0.2254 = 22.54%；收入为 0 时给 0。 */
  margin: string
  /** 有销售行没有可用成本（没挂采购单或采购单还是草稿），成本与毛利因此不完整。 */
  cost_missing: boolean
  /** 成本 / 运杂费 / 毛利被服务端清掉（T-36）：收入照给。 */
  cost_hidden?: boolean
}

export interface Payment {
  id: number
  doc_no: string
  /** 10 收款 / 20 付款。方向决定能核应收还是应付。 */
  direction: number
  partner_id: number
  /** YYYY-MM-DD：钱动的那天，不是录入日。 */
  pay_date: string
  currency_code: string
  /** 金额与汇率是定点字符串：前端只展示，算术在服务端。 */
  exchange_rate: string
  amount: string
  amount_base: string
  /** TT / LC / DP / DA / OA，与主体的默认结算方式同源。 */
  method: string
  /** 银行水单号，外部号：只记录、不参与发号与唯一性。 */
  reference: string
  remark: string
  partner?: Partner | null
  allocations?: PaymentAllocation[]
  created_at: string
  updated_at: string
}

/**
 * 报关单：一票货的海关申报记录。
 *
 * **一票可以有多张**：不同 HS 编码要分票申报，被退单后改单重报也是新的一条，
 * 旧的留着——「海关退回过什么」只在旧单还在的时候才回答得了。
 *
 * 不复用 DocumentsStatus：报关单没有「备货中」「已出货」，
 * 而它的「退单」也不等于作废，改完还要重新申报。
 */
export interface CustomsDeclaration {
  id: number
  shipment_id: number
  /** 10 待申报 / 20 已申报 / 30 已放行 / 90 已退单 */
  status: number
  /** 海关给的 18 位号，外部号：只记录、不参与发号与唯一性。可为空（待申报时还没有）。 */
  decl_no: string
  decl_date: string | null
  /** 监管方式代码，如 0110 一般贸易 / 0615 进料对口 / 1039 市场采购。 */
  supervision_mode: string
  remark: string
  created_at: string
  updated_at: string
}

/** 出货单新建页的选行来源：还能出货的销售订单行。 */
export interface ShipmentCandidate {
  sales_order_id: number
  sales_order_doc_no: string
  partner_name: string
  sales_order_item_id: number
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: string
  shipped_qty: string
  unshipped_qty: string
  delivery_date?: string
}

/**
 * 库存流水：库存的唯一事实来源。
 *
 * qty 永远是正数，出入看 direction（1 入 / -1 出）；
 * type 只说明"为什么动"，不决定方向——调拨与盘点都有两个方向。
 */
export interface StockMovement {
  id: number
  doc_no: string
  product_id: number
  /** 产品快照：产品改名后回看历史流水仍是当时的说法。 */
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  type: number
  direction: number
  qty: string
  /** 业务发生日，不是录入时间。 */
  occurred_at: string
  source_type: string
  source_id: number
  source_doc_no: string
  remark: string
  created_at: string
  updated_at: string
}

/**
 * 库存行（快照）。只读：库存只能由流水推出来，界面没有直接改它的入口。
 */
export interface StockSnapshotRow {
  product_id: number
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: string
  last_occurred_at?: string | null
  updated_at: string
}

/**
 * 出入库单明细。
 *
 * qty 的含义看单据类型：入库 / 出库是数量，盘点是**实盘数**。
 * book_qty 只在盘点单确认时写入（当时的账面），差异 = qty − book_qty。
 */
export interface StockDocumentItem {
  id: number
  document_id: number
  product_id: number
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: string
  book_qty?: string | null
  remark: string
}

/**
 * 出入库单。
 *
 * 草稿不动库存；确认那一刻才按类型生成流水。确认之后不可改不可删，
 * 要纠正就做一张反向单。
 */
export interface StockDocument {
  id: number
  doc_no: string
  /** 10 采购入库 / 20 销售出库 / 40 盘点 / 50 采购退货（30 调拨预留，尚未实现）。 */
  type: number
  status: number
  doc_date: string
  partner_id?: number | null
  partner?: Partner | null
  /** 来源单号（PO-… / SH-…），仅追溯。 */
  source_doc_no: string
  remark: string
  posted_at?: string | null
  items?: StockDocumentItem[]
  created_at: string
  updated_at: string
}

/** 重算结果：正常情况下 mismatched 为空，非空说明有人绕过流水改过快照。 */
export interface StockRecalcResult {
  recalculated: number
  mismatched: {
    product_id: number
    snapshot_qty: string
    movement_qty: string
  }[]
}

/**
 * 采购明细行。
 *
 * sales_order_item_id 非空表示这一行是为某张销售订单的某一行采购的（以销定采）；
 * source_id 记录这行对应哪条货源，仅作追溯——采购价是谈出来的，不从货源表读。
 */
export interface PurchaseOrderItem {
  id: number
  purchase_order_id: number
  product_id: number
  sku_code: string
  name_cn: string
  spec: string
  unit: string
  qty: string
  unit_price: string
  amount: string
  amount_base: string
  sales_order_item_id: number | null
  source_id: number | null
  remark: string
  product?: Product | null
}

/**
 * 采购订单：销售订单的镜像。对方是供应商，采购币种通常与销售币种不同，
 * 所以汇率单独锁一份；合计同样只由明细汇总。
 */
export interface PurchaseOrder {
  id: number
  doc_no: string
  supplier_id: number
  trade_term: string
  status: number
  order_date: string
  /** 供应商承诺的到货日，可空。 */
  delivery_date?: string | null
  currency_code: string
  exchange_rate: string
  amount_total: string
  amount_base_total: string
  remark: string
  supplier?: Partner | null
  items?: PurchaseOrderItem[]
  /**
   * 跟单节点（T-38），只有详情带。
   *
   * 挂在这一单的**行**上：一张单常有好几个 SKU、分批从不同工厂出来，
   * "生产完成了"对整张单没有意义。
   */
  milestones?: PurchaseMilestone[]
  /**
   * 列表投影出来的「最靠前的逾期未完成节点」文案（如「验货」），没有逾期时为空。
   * 只读，后端按截止日算，前端不要自己推导。
   */
  overdue_node?: string
  created_at: string
  updated_at: string
}

/** 采购跟单的五个节点（T-38）。编号留间隔，便于将来在中间插节点。 */
export const PURCHASE_NODES = [
  { value: 10, label: '下单' },
  { value: 20, label: '生产完成' },
  { value: 30, label: '验货' },
  { value: 40, label: '入仓' },
  { value: 50, label: '出运' },
] as const

/**
 * 采购订单行上的一个跟单节点。
 *
 * `plan_date` 与 `done_date` 分开：只有完成日答不了「该做而没做」（逾期就是这么算的），
 * 只有计划日答不了「到底做完没有」。`done_date` 为空表示还没做。
 */
export interface PurchaseMilestone {
  id: number
  purchase_order_id: number
  purchase_order_item_id: number
  /** 10 下单 / 20 生产完成 / 30 验货 / 40 入仓 / 50 出运 */
  node: number
  plan_date: string | null
  done_date: string | null
  /** 补录完成的人；只排计划时为空。 */
  operator: string
  remark: string
}

export interface QuotationPreviewItem {
  product_id: number
  sku_code: string
  name_cn: string
  unit: string
  qty: string
  unit_price: string
  amount: string
  amount_base: string
  cost_price: string
  cost_amount: string
  gross_profit: string
  source_id: number | null
  source_name: string
}

/**
 * 产品编号：SKU 之外的别名（客户物料号、供应商货号、工厂号、旧 SKU）。
 *
 * 不参与发号、不进单据与打印件——它只服务「客户报一个号也能找到货」。
 * code 是原样显示，code_norm 是检索用的归一化形态（服务端算，前端只读）。
 */
export interface ProductCode {
  id: number
  product_id: number
  code: string
  code_norm: string
  /** 见 model.ProductCodeSource*：sku / customer / supplier / factory / other */
  source: string
  remark: string
  created_at: string
  updated_at: string
}

/**
 * 附件：挂在主体或单据上的文件元数据（提单、报关单扫描件、验货报告）。
 *
 * 没有内容字段：列表与详情只看元数据，下载走 `/attachments/:id/download`。
 * entity_type 的取值与后端 model.Attach* 常量一致，拼错的表现是 404。
 */
export interface Attachment {
  id: number
  entity_type: string
  entity_id: number
  file_name: string
  /** 服务端嗅探出来的类型，不是上传时声明的。 */
  mime_type: string
  size: number
  sha256: string
  /** 上传人 id；内部任务上传时为 0。 */
  uploaded_by: number
  remark: string
  created_at: string
  updated_at: string
}

/** 报价试算结果：口径与保存后完全一致（同一个折算函数）。 */
export interface QuotationPreview {
  currency_code: string
  exchange_rate: string
  items: QuotationPreviewItem[]
  amount_total: string
  amount_base_total: string
  cost_total: string
  gross_profit: string
  margin_rate: string
}
