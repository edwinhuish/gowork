import { forwardRef } from 'react'
import type { TooltipProps } from 'antd'
import { Tooltip as AntdTooltip } from 'antd'

/**
 * 全局统一的 Tooltip 封装。
 *
 * antd 的 Tooltip 默认只做翻转（autoAdjustOverflow = { adjustX, adjustY }），
 * 浮层贴到视口边缘时距边缘为 0 —— 顶部右侧的全屏按钮这类边缘触发器就会"紧贴右边距"，
 * 缩放窗口或窄屏下尤其明显。这里在默认值里补上 shiftX / shiftY，让浮层始终与
 * 视口边缘保留 8px 间距；翻转（adjustX/adjustY）照旧，所以靠近边缘时仍会自动
 * 换到另一侧、不会被判掉。其余 props 原样透传，调用方无感。
 *
 * 单独成组件而不是在几十个文件里逐个加 prop：一套行为只定义一次，将来要调间距
 * 改这一处即可。
 *
 * 注：shiftX / shiftY 在 antd 运行时支持（见 rc-trigger 的 getOverflowOptions），
 * 但不在公开类型 AdjustOverflow 里，故用断言写出。
 */
const DEFAULT_OVERFLOW = {
  adjustX: 1,
  adjustY: 1,
  shiftX: 8,
  shiftY: 8,
} as TooltipProps['autoAdjustOverflow']

export const Tooltip = forwardRef<unknown, TooltipProps>((props, ref) => {
  const { autoAdjustOverflow, ...rest } = props
  return (
    <AntdTooltip
      ref={ref as never}
      autoAdjustOverflow={autoAdjustOverflow ?? DEFAULT_OVERFLOW}
      {...rest}
    />
  )
})

Tooltip.displayName = 'Tooltip'
