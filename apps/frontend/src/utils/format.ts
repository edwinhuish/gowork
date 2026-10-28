/** 时间格式化：后端返回 RFC3339 字符串，统一成本地可读格式。 */
export function formatTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
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
export function formatPercent(value?: number | null, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return `${(value * 100).toFixed(digits)}%`
}

/**
 * 数量按 3 位小数去掉尾零。
 *
 * 后端数量存 decimal(14,3)，直接显示会得到「800.000」，而仓库与货代看单时
 * 会把它读成"有小数"；去掉尾零与后端报错提示里的写法也保持一致。
 */
export function formatQty(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return value.toFixed(3).replace(/\.?0+$/, '')
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
