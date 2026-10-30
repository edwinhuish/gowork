/**
 * 在两个颜色之间插值。
 *
 * 存在的理由只有一个：**有序量**需要同一色相的深浅梯度，而语义色只有五档，
 * 给不出梯度。逾期分桶（1-30 / 31-60 / 61-90 / 90+）就是典型：四档用四个
 * 色相（gold / orange / volcano / red）会读成四种状态，而它们明明是同一个
 * 量在变大。在 warning 与 error 之间插值拿中间档，色号仍然只有 theme.ts
 * 里那几个，不新增一套调色板。
 *
 * `weight` 是 `from` 的占比：0 得到 `to`，1 得到 `from`。
 */
export function mixHex(from: string, to: string, weight: number): string {
  const a = parse(from)
  const b = parse(to)
  const w = Math.min(1, Math.max(0, weight))
  const channel = (x: number, y: number) => Math.round(x * w + y * (1 - w))
  const hex = (v: number) => Math.min(255, Math.max(0, v)).toString(16).padStart(2, '0')
  return `#${hex(channel(a[0], b[0]))}${hex(channel(a[1], b[1]))}${hex(channel(a[2], b[2]))}`
}

/** 只取前六位：antd 的 token 有时带回 alpha（`#rrggbbaa`），混色用不到。 */
function parse(color: string): [number, number, number] {
  let h = color.trim().replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  h = h.slice(0, 6).padEnd(6, '0')
  const n = Number.parseInt(h, 16)
  if (Number.isNaN(n)) return [0, 0, 0]
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}
