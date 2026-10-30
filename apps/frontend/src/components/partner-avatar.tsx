import type { CSSProperties } from 'react'

import { useAuthedUrl } from './authed-image'

/**
 * 主体头像 / Logo。
 *
 * 三种来源按顺序降级，都收敛在这一个组件里：
 *   1. 后端回填的 `logo_url`（最新一张图片附件）能取到字节 → 显示真图；
 *   2. 没有任何 Logo、或取图失败 → 显示名称占位标识（见 partnerInitials）；
 *   3. 图还在路上 → 也显示占位标识，加载完才换成图。
 *
 * 第 3 条是有意的：占位与图同宽同高，图片到了不会让表格行或卡片标题跳动。
 * 列表视图与卡片视图调用的必须是**这一个组件**，否则"卡片有图、列表没图"
 * 会当场露出来，而排查时谁也不会怀疑是视图切换的问题。
 *
 * 取图走 useAuthedUrl（fetch → blob → object URL）而不是 `<img src>`：
 * 取图接口要 Authorization，直接写进 src 只会拿到 401。与产品图片同一条通道。
 */

/** 默认尺寸。列表列与卡片头部都用这个值，改它等于同时改两个视图。 */
export const PARTNER_AVATAR_SIZE = 40

/**
 * 占位标识的底色。
 *
 * 全是偏深的色号（白字压得住），由名称哈希选：同一个主体的颜色在任何页面、
 * 任何一次渲染都一致，所以它是"认人"的辅助线索而不是随机装饰。
 */
const PALETTE = [
  '#0b5cad',
  '#08979c',
  '#237804',
  '#a8071a',
  '#531dab',
  '#d4380d',
  '#ad4e00',
  '#391085',
]

/** 中文 / 日文 / 韩文所在的码区：这些语言的"首字母"没有意义，取首文字。 */
const CJK = /[぀-ヿ㐀-䶿一-鿿豈-﫿가-힯]/

/**
 * 名称 → 占位标识。
 *
 * 中文取首文字（"上海远东贸易"→"上"），西文取前两个词的首字母
 * （"Ningbo Sunrise Trading"→"NS"）：只取第一个字母的话，同城同行的客户
 * 十有八九都是同一个字母，占位标识就成了没有区分度的色块。
 *
 * 名称为空返回 `?` 而不是空串：空心的圆点在列表里会被当成"没加载出来"。
 */
export function partnerInitials(name?: string): string {
  const trimmed = (name ?? '').trim()
  if (!trimmed) return '?'

  const chars = Array.from(trimmed)
  if (CJK.test(chars[0] ?? '')) return chars[0]!

  const words = trimmed.split(/[\s\-_/&.,()]+/).filter(Boolean)
  if (words.length === 0) return (chars[0] ?? '?').toUpperCase()
  return words
    .slice(0, 2)
    .map((word) => Array.from(word)[0]!.toUpperCase())
    .join('')
}

/** 字符串 → 调色板下标。djb2 的位移版，够稳也不必引入哈希库。 */
function paletteIndex(seed: string): number {
  let hash = 5381
  for (let i = 0; i < seed.length; i += 1) {
    hash = ((hash << 5) + hash + seed.charCodeAt(i)) | 0
  }
  return Math.abs(hash) % PALETTE.length
}

interface PartnerAvatarProps {
  /** 名称：既是占位标识的来源，也是 alt / 悬浮提示。 */
  name: string
  /** 后端回填的取图路径；没有就不传，不必为了占位把参数写成空串。 */
  logoUrl?: string
  /** 边长（px）。两个视图都用默认值，只有在确实更大的容器里才覆盖。 */
  size?: number
}

export function PartnerAvatar({ name, logoUrl, size = PARTNER_AVATAR_SIZE }: PartnerAvatarProps) {
  const { url, failed } = useAuthedUrl(logoUrl)
  const box: CSSProperties = { width: size, height: size }

  if (logoUrl && url && !failed) {
    return (
      <span className="partner-avatar" style={box}>
        <img src={url} alt={name} className="partner-avatar-img" />
      </span>
    )
  }

  return (
    <span
      className="partner-avatar"
      style={{
        ...box,
        background: PALETTE[paletteIndex(name)],
        fontSize: Math.round(size * 0.38),
      }}
      role="img"
      aria-label={name}
      title={name}
    >
      {partnerInitials(name)}
    </span>
  )
}
