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
 */
export const brand = {
  primary: '#2563eb',
  primaryHover: '#1d4ed8',
  primarySoft: '#eff4ff',
  background: '#f4f6fa',
  surface: '#ffffff',
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
}

/** 全站统一圆角。表格、卡片、按钮、输入框共用一套档位，避免圆角混搭。 */
const RADIUS = 8

/** 卡片浮起：两段极浅阴影，比加边框轻，又能在 #f4f6fa 底上分出层次。 */
const SHADOW_CARD = '0 1px 2px rgba(16, 24, 40, 0.05), 0 1px 3px rgba(16, 24, 40, 0.04)'

export const themeConfig: ThemeConfig = {
  // 开 CSS 变量：结构性样式可以引用 --ant-*，色值仍然只有一个来源。
  cssVar: true,

  token: {
    colorPrimary: brand.primary,
    colorLink: brand.primary,
    colorSuccess: brand.success,
    colorWarning: brand.warning,
    colorError: brand.danger,
    colorInfo: brand.info,
    colorBgLayout: brand.background,
    colorBgContainer: brand.surface,
    colorBgElevated: brand.surface,
    colorBorder: brand.border,
    colorBorderSecondary: brand.border,
    colorText: brand.text,
    colorTextSecondary: brand.textMuted,
    colorTextTertiary: brand.textFaint,
    colorTextDescription: brand.textMuted,
    colorFillSecondary: '#eef2f8',
    colorFillTertiary: '#f4f6fa',
    colorFillQuaternary: '#f8fafc',
    borderRadius: RADIUS,
    borderRadiusLG: RADIUS + 2,
    borderRadiusSM: RADIUS - 2,
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontSize: 14,
    controlHeight: 34,
    wireframe: false,
    boxShadow: '0 6px 16px rgba(16, 24, 40, 0.08)',
    boxShadowSecondary: '0 6px 20px rgba(16, 24, 40, 0.10)',
    boxShadowTertiary: SHADOW_CARD,
  },

  components: {
    Layout: {
      siderBg: brand.surface,
      headerBg: brand.surface,
      bodyBg: brand.background,
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
     */
    Menu: {
      itemBg: 'transparent',
      subMenuItemBg: 'transparent',
      itemColor: brand.textMuted,
      itemHoverColor: brand.text,
      itemHoverBg: '#f1f5fb',
      itemSelectedBg: brand.primarySoft,
      itemSelectedColor: brand.primary,
      itemActiveBg: '#e8efff',
      groupTitleColor: brand.textFaint,
      groupTitleFontSize: 12.5,
      itemHeight: 38,
      itemMarginInline: 8,
      itemBorderRadius: RADIUS - 2,
      subMenuItemBorderRadius: RADIUS - 2,
    },

    // 表头用浅灰区分于数据行，比加边框更轻；行高给足，扫读密集数据时不挤。
    Table: {
      headerBg: brand.surfaceMuted,
      headerColor: brand.textMuted,
      headerSplitColor: 'transparent',
      rowHoverBg: brand.primarySoft,
      cellPaddingBlock: 12,
      borderColor: brand.border,
    },

    Card: {
      headerFontSize: 15,
      headerHeight: 50,
      paddingLG: 20,
    },

    Button: {
      primaryShadow: 'none',
      defaultShadow: 'none',
      fontWeight: 500,
    },

    Descriptions: {
      labelBg: brand.surfaceMuted,
      titleMarginBottom: 12,
    },

    Form: {
      labelColor: brand.textMuted,
      verticalLabelPadding: '0 0 4px',
    },

    // 概览与报表的抬头数字：标题压小、数字加大，一屏能读到更多块。
    Statistic: {
      titleFontSize: 13,
      contentFontSize: 26,
    },

    Tag: {
      defaultBg: '#f2f5fa',
      defaultColor: brand.textMuted,
    },

    Breadcrumb: {
      itemColor: brand.textMuted,
      lastItemColor: brand.text,
      separatorColor: brand.textFaint,
      linkColor: brand.textMuted,
      linkHoverColor: brand.primary,
      fontSize: 13,
    },

    Tabs: {
      titleFontSize: 14,
      horizontalItemPadding: '10px 0',
    },

    Segmented: {
      itemSelectedBg: brand.surface,
      trackBg: '#eef2f8',
      itemSelectedColor: brand.text,
    },

    Alert: {
      borderRadius: RADIUS,
    },
  },
}
