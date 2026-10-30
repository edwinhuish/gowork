import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * 设计 token。
 *
 * 这套取值不是新拍脑袋的，而是把原来散落在 index.css 里的硬编码颜色
 * （--primary / --sidebar / --bg / --radius）收敛成单一来源：
 * 改造前前端有 8 个 CSS 变量 + 各页面内联样式，同一类灰出现过 4 个近似值。
 * 现在语义只在这里定义一次，由 antd 的 ConfigProvider 下发到所有组件。
 *
 * 开了 cssVar 之后这些值同时会变成 `--ant-*` 变量，index.css 里那些 antd 覆盖不到的
 * 结构性样式（卡片浮起、页头分隔、单据链）就能直接引用，不必在 CSS 里再抄一份色值。
 *
 * 亮色与暗色是两套语义色板，不是把亮色反相：暗底上的主色必须提亮才够对比，
 * 而 `primarySoft` 这类"浅底"在暗色下要变成带透明度的主色，否则会变成一块亮斑。
 */
export type ColorMode = 'light' | 'dark'

export const palette = {
  light: {
    primary: '#2563eb',
    primaryHover: '#1d4ed8',
    primarySoft: '#eff4ff',
    background: '#f4f6fa',
    surface: '#ffffff',
    /** 弹层与下拉：比内容底再亮一档，浮起关系才看得出来。 */
    surfaceElevated: '#ffffff',
    surfaceMuted: '#f8fafc',
    border: '#e3e8f0',
    text: '#1c2532',
    textMuted: '#6b7788',
    /** 第三级文字：占位、单位、口径说明。比 textMuted 更淡，但仍要能读。 */
    textFaint: '#98a3b3',
    success: '#16a34a',
    warning: '#d97706',
    danger: '#dc2626',
    /** 信息蓝，用于"进行中"这类中性状态，与主色区分开。 */
    info: '#0ea5e9',
    fillSecondary: '#eef2f8',
    fillTertiary: '#f4f6fa',
    fillQuaternary: '#f8fafc',
    shadow: '0 6px 16px rgba(16, 24, 40, 0.08)',
    shadowSecondary: '0 6px 20px rgba(16, 24, 40, 0.10)',
    shadowCard: '0 1px 2px rgba(16, 24, 40, 0.05), 0 1px 3px rgba(16, 24, 40, 0.04)',
  },
  dark: {
    primary: '#4d8df7',
    primaryHover: '#6ba1ff',
    primarySoft: 'rgba(77, 141, 247, 0.16)',
    background: '#0f141b',
    surface: '#161c25',
    surfaceElevated: '#1e2632',
    surfaceMuted: '#1a212b',
    border: '#2a3444',
    text: '#e7ecf4',
    textMuted: '#9aa7b8',
    textFaint: '#6d7b8d',
    success: '#2ecc71',
    warning: '#f0a020',
    danger: '#f6705f',
    info: '#3ec2f0',
    fillSecondary: '#202834',
    fillTertiary: '#1a212b',
    fillQuaternary: '#151b23',
    shadow: '0 6px 16px rgba(0, 0, 0, 0.45)',
    shadowSecondary: '0 8px 24px rgba(0, 0, 0, 0.5)',
    shadowCard: '0 1px 2px rgba(0, 0, 0, 0.4)',
  },
} as const

/** 亮色语义色板，index.css 与旧引用仍按这个名字取色。 */
export const brand = palette.light

/** 全站统一圆角。表格、卡片、按钮、输入框共用一套档位，避免圆角混搭。 */
const RADIUS = 8

/** 表格与卡片的行高/内距，密度切换时按比例收一档。 */
const density = (compact: boolean) => ({
  cellPaddingBlock: compact ? 8 : 12,
  cardPadding: compact ? 14 : 20,
  headerHeight: compact ? 46 : 50,
  formItemGap: compact ? 14 : 20,
  filterPadding: compact ? '10px 12px' : '14px 16px',
})

/**
 * 按「亮/暗 × 标准/紧凑」生成主题。
 *
 * 只用 antd 的三个 algorithm 组合，不手写尺寸梯度：compactAlgorithm 会把
 * 控件高度、行距、字号一起收档，手写一份反而会在后续升级时和组件库对不上。
 */
export function buildThemeConfig({
  mode = 'light',
  compact = false,
}: {
  mode?: ColorMode
  compact?: boolean
} = {}): ThemeConfig {
  const c = palette[mode]
  const d = density(compact)

  return {
    // 开 CSS 变量：结构性样式可以引用 --ant-*，色值仍然只有一个来源。
    cssVar: true,
    algorithm: [
      mode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
      ...(compact ? [theme.compactAlgorithm] : []),
    ],

    token: {
      colorPrimary: c.primary,
      colorLink: c.primary,
      colorSuccess: c.success,
      colorWarning: c.warning,
      colorError: c.danger,
      colorInfo: c.info,
      colorBgLayout: c.background,
      colorBgContainer: c.surface,
      colorBgElevated: c.surfaceElevated,
      colorBorder: c.border,
      colorBorderSecondary: c.border,
      colorText: c.text,
      colorTextSecondary: c.textMuted,
      colorTextTertiary: c.textFaint,
      colorTextDescription: c.textMuted,
      colorTextQuaternary: c.textFaint,
      colorFillSecondary: c.fillSecondary,
      colorFillTertiary: c.fillTertiary,
      colorFillQuaternary: c.fillQuaternary,
      borderRadius: RADIUS,
      borderRadiusLG: RADIUS + 2,
      borderRadiusSM: RADIUS - 2,
      fontFamily:
        "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
      fontSize: 14,
      /*
       * 字阶。
       *
       * 只有 h4 / h5 真正出现在界面上（页头标题与区块标题），h1–h3 是给
       * Result / Drawer 这类偶尔用一次的组件兜底的，所以这里只对前两级做细调：
       * 页头 18px、区块 15px，都比 antd 默认小一档——一眼要能同时读到
       * "这是哪页"和"这一屏有什么数据"，标题占掉太多行距会压迫数据区。
       *
       * 正文行高给到 1.6 而不是默认的 1.571：界面里中英混排，中文方块字在
       * 密排行距下会显脏。西文的 -apple-system 与 PingFang 度量不同，
       * 这个值对两边都不难看。
       */
      lineHeight: 1.6,
      fontSizeHeading1: 32,
      fontSizeHeading2: 24,
      fontSizeHeading3: 20,
      fontSizeHeading4: 18,
      fontSizeHeading5: 15,
      lineHeightHeading1: 1.25,
      lineHeightHeading2: 1.28,
      lineHeightHeading3: 1.32,
      lineHeightHeading4: 1.35,
      lineHeightHeading5: 1.4,
      // 间距基准 4：sizeStep / sizeUnit 一起收，Grid gutter、Flex gap 才有同一个点。
      sizeStep: 4,
      sizeUnit: 4,
      controlHeight: 34,
      wireframe: false,
      boxShadow: c.shadow,
      boxShadowSecondary: c.shadowSecondary,
      boxShadowTertiary: c.shadowCard,
      // 动效统一：交互反馈既要有，又不能拖沓。尊重系统的"减少动效"由 antd 处理。
      motionDurationFast: '0.1s',
      motionDurationMid: '0.18s',
      motionDurationSlow: '0.26s',
    },

    components: {
      Layout: {
        siderBg: c.surface,
        headerBg: c.surface,
        bodyBg: c.background,
        headerHeight: 56,
        headerPadding: '0 20px',
      },

      /*
       * 侧栏是浅色的。
       *
       * 之前这里配的是 darkItem*，但它们从来没生效过：@refinedev/antd 的 ThemedSider
       * 把 `backgroundColor: token.colorBgElevated` 内联写进 Layout.Sider，Menu 也没传
       * theme="dark"，所以侧栏其实是白底 + 浅色菜单 —— 而品牌块的文字是近白色，
       * 实际渲染出来是白底白字（只剩那个蓝色方块看得见）。
       *
       * 与其用 !important 去压内联样式改回深色，不如顺着它做成浅色侧栏：
       * 选中项用主色的浅底 + 主色文字，层级比整块深色底更轻，也不会和内容区抢注意力。
       * 暗色模式下 colorBgElevated 变深，侧栏自动跟着变深，不需要另开分支。
       */
      Menu: {
        itemBg: 'transparent',
        subMenuItemBg: 'transparent',
        itemColor: c.textMuted,
        itemHoverColor: c.text,
        itemHoverBg: c.fillTertiary,
        itemSelectedBg: c.primarySoft,
        itemSelectedColor: c.primary,
        itemActiveBg: c.primarySoft,
        groupTitleColor: c.textFaint,
        groupTitleFontSize: 12.5,
        itemHeight: compact ? 34 : 38,
        itemMarginInline: 8,
        itemBorderRadius: RADIUS - 2,
        subMenuItemBorderRadius: RADIUS - 2,
      },

      // 表头用浅灰区分于数据行，比加边框更轻；行高给足，扫读密集数据时不挤。
      Table: {
        headerBg: c.surfaceMuted,
        headerColor: c.textMuted,
        headerSplitColor: 'transparent',
        rowHoverBg: c.primarySoft,
        cellPaddingBlock: d.cellPaddingBlock,
        borderColor: c.border,
        headerBorderRadius: RADIUS,
      },

      Card: {
        headerFontSize: 15,
        headerHeight: d.headerHeight,
        paddingLG: d.cardPadding,
      },

      Button: {
        primaryShadow: 'none',
        defaultShadow: 'none',
        fontWeight: 500,
      },

      Descriptions: {
        labelBg: c.surfaceMuted,
        titleMarginBottom: 12,
        itemPaddingBottom: 10,
      },

      Form: {
        labelColor: c.textMuted,
        labelFontSize: 13,
        verticalLabelPadding: '0 0 4px',
        itemMarginBottom: d.formItemGap,
      },

      // 概览与报表的抬头数字：标题压小、数字加大，一屏能读到更多块。
      Statistic: {
        titleFontSize: 13,
        contentFontSize: 26,
      },

      Tag: {
        defaultBg: c.fillSecondary,
        defaultColor: c.textMuted,
        borderRadiusSM: 6,
      },

      Breadcrumb: {
        itemColor: c.textMuted,
        lastItemColor: c.text,
        separatorColor: c.textFaint,
        linkColor: c.textMuted,
        linkHoverColor: c.primary,
        fontSize: 13,
      },

      Tabs: {
        titleFontSize: 14,
        horizontalItemPadding: '10px 0',
      },

      Segmented: {
        itemSelectedBg: c.surfaceElevated,
        trackBg: c.fillSecondary,
        itemSelectedColor: c.text,
      },

      Alert: {
        borderRadius: RADIUS,
      },

      Modal: {
        titleFontSize: 16,
        headerBg: c.surfaceElevated,
        contentBg: c.surfaceElevated,
      },

      Drawer: {
        footerPaddingBlock: 12,
        footerPaddingInline: 20,
      },

      // 分页条与表格同宽时，页码块略小一档，不让它比数据行更抢眼。
      Pagination: {
        itemSize: compact ? 28 : 32,
      },

      Input: {
        paddingBlock: compact ? 4 : 6,
        activeShadow: `0 0 0 3px ${c.primarySoft}`,
        hoverBorderColor: c.primary,
      },

      InputNumber: {
        activeShadow: `0 0 0 3px ${c.primarySoft}`,
      },

      Select: {
        optionSelectedBg: c.primarySoft,
        optionSelectedColor: c.primary,
        optionSelectedFontWeight: 500,
        optionActiveBg: c.fillTertiary,
        optionPadding: '6px 12px',
      },

      Dropdown: {
        paddingBlock: 6,
      },

      Tooltip: {
        colorBgSpotlight: mode === 'dark' ? '#2b3646' : '#1c2532',
      },

      Popover: {
        titleMinWidth: 120,
      },

      Empty: {
        colorTextDescription: c.textFaint,
      },

      Steps: {
        titleLineHeight: 26,
        iconSize: 26,
      },

      Timeline: {
        itemPaddingBottom: 16,
      },

      Progress: {
        defaultColor: c.primary,
        remainingColor: c.fillSecondary,
      },

      Checkbox: {
        borderRadiusSM: 4,
      },

      Message: {
        contentPadding: '10px 16px',
      },
    },
  }
}

/** 亮色 + 标准密度的默认主题：登录页等挂不到偏好 Provider 之外的地方直接用。 */
export const themeConfig = buildThemeConfig()
