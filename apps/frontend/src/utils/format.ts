/** 时间格式化：后端返回 RFC3339 字符串，统一成本地可读格式。 */
export function formatTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}
