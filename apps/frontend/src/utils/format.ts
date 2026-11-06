/**
 * 时间格式化：后端返回 RFC3339 字符串，统一本地时区的「2026-10-30 20:08:04」。
 *
 * 不用 toLocaleString('zh-CN')：它给的是「2026/10/30 20:08:04」——
 * 一是同一张详情页里，业务日期是 2026-10-30、录入时间是 2026/10/30，
 * 两种分隔符；二是月日不补零，日志这类右对齐的长列表里一眼看得出参差。
 * 补零之后固定 19 个字符，列宽也好算。
 */
export function formatTime(value?: string) {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/**
 * 业务日期：只取日期部分，不做时区换算。
 *
 * 汇率生效日这类字段后端存的是 UTC 零点，用 new Date() 转本地时间会
 * 在 UTC-x 时区显示成前一天 —— 对"某天生效的价"来说是错的。
 */
export function formatDate(value?: string) {
  if (!value) return '—'
  return value.slice(0, 10)
}

/**
 * 比例转百分数。后端存比例（0.235），百分数在计算里每次都要除 100，
 * 漏一处就是差一百倍，所以只在显示这一层转。
 */
export function formatPercent(value?: number | string | null, digits = 2) {
  // 定点小数以字符串下发（比例只在显示这一层乘 100）。
  const num = typeof value === 'string' ? Number(value) : value
  if (num === null || num === undefined || Number.isNaN(num)) return '—'
  return `${(num * 100).toFixed(digits)}%`
}

/**
 * 数量按 3 位小数去掉尾零。
 *
 * 后端数量存 decimal(14,3)，直接显示会得到「800.000」，而仓库与货代看单时
 * 会把它读成"有小数"；去掉尾零与后端报错提示里的写法也保持一致。
 */
export function formatQty(value?: number | string | null) {
  if (value === null || value === undefined) return '—'
  // 定点数量以字符串下发，这里只做显示，转 number 只为去掉尾零。
  const num = typeof value === 'string' ? Number(value) : value
  if (Number.isNaN(num)) return '—'
  return num.toFixed(3).replace(/\.?0+$/, '')
}

/**
 * 把若干行的数量**按单位**汇总，例如「8400 PCS · 2400 SET」。
 *
 * 不同单位不能直接相加：一票里 6000 PCS + 2400 SET + 3000 PCS 报「11400」
 * 是没有意义的数——单位是数量的一部分，PCS 与 SET 相加等于把两种东西折成
 * 一个不存在的单位。合计先按单位分组，再各自求和。
 *
 * 单位为空时按「件」兜底：老数据的明细行可能没带单位，空字符串进到
 * 「8400 」这种显示里，看起来像漏了东西。
 */
export function formatQtyByUnit(rows: { qty: string; unit?: string | null }[]) {
  const byUnit = new Map<string, number>()
  for (const row of rows) {
    const unit = row.unit?.trim() || '件'
    byUnit.set(unit, (byUnit.get(unit) ?? 0) + Number(row.qty))
  }
  return [...byUnit.entries()]
    .map(([unit, qty]) => `${formatQty(qty)} ${unit}`)
    .join(' · ')
}

/**
 * 文件大小（字节 → KB / MB）。
 *
 * 附件列表要知道"那份扫描件多大"。用 1024 进制并保留一位小数：
 * 这里要的是量级，10MB 与 10.4MB 的差别没有意义。
 */
export function formatBytes(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value) || value < 0) return '—'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
