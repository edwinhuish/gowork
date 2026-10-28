import type { ThemeConfig } from 'antd'

/**
 * 设计 token。
 *
 * 这套取值不是新拍脑袋的，而是把原来散落在 index.css 里的硬编码颜色
 * （--primary / --sidebar / --bg / --radius）收敛成单一来源：
 * 改造前前端有 8 个 CSS 变量 + 各页面内联样式，同一类灰出现过 4 个近似值。
 * 现在语义只在这里定义一次，由 antd 的 ConfigProvider 下发到所有组件。
 */
export const brand = {
  primary: '#2563eb',
  primaryHover: '#1d4ed8',
  primarySoft: '#eff4ff',
  background: '#f4f6fa',
  surface: '#ffffff',
  border: '#e3e8f0',
  text: '#1c2532',
  textMuted: '#6b7788',
  sider: '#131c2b',
  siderActive: '#1e2b3f',
  siderText: '#a8b6c9',
  success: '#16a34a',
  warning: '#d97706',
  danger: '#dc2626',
}

/** 全站统一圆角。表格、卡片、按钮、输入框共用一套档位，避免圆角混搭。 */
const RADIUS = 8

export const themeConfig: ThemeConfig = {
  token: {
    colorPrimary: brand.primary,
    colorLink: brand.primary,
    colorSuccess: brand.success,
    colorWarning: brand.warning,
    colorError: brand.danger,
    colorBgLayout: brand.background,
    colorBgContainer: brand.surface,
    colorBorder: brand.border,
    colorBorderSecondary: brand.border,
    colorText: brand.text,
    colorTextSecondary: brand.textMuted,
    borderRadius: RADIUS,
    borderRadiusLG: RADIUS + 2,
    borderRadiusSM: RADIUS - 2,
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontSize: 14,
    controlHeight: 34,
    wireframe: false,
  },

  components: {
    Layout: {
      siderBg: brand.sider,
      headerBg: brand.surface,
      bodyBg: brand.background,
      headerHeight: 56,
      headerPadding: '0 20px',
    },

    // 侧边栏是深色的，Menu 必须走 dark 主题 token，否则浅色菜单项在深底上看不清。
    Menu: {
      darkItemBg: brand.sider,
      darkSubMenuItemBg: brand.sider,
      darkItemColor: brand.siderText,
      darkItemHoverBg: brand.siderActive,
      darkItemSelectedBg: brand.primary,
      darkItemSelectedColor: '#ffffff',
      itemHeight: 40,
      itemMarginInline: 8,
      itemBorderRadius: RADIUS - 2,
    },

    // 表头用浅灰区分于数据行，比加边框更轻；行高给足，扫读密集数据时不挤。
    Table: {
      headerBg: '#f8fafc',
      headerColor: brand.textMuted,
      headerSplitColor: 'transparent',
      rowHoverBg: brand.primarySoft,
      cellPaddingBlock: 12,
      borderColor: brand.border,
    },

    Card: {
      headerFontSize: 15,
      paddingLG: 20,
    },

    Button: {
      primaryShadow: 'none',
      defaultShadow: 'none',
      fontWeight: 500,
    },

    Descriptions: {
      labelBg: '#f8fafc',
      titleMarginBottom: 12,
    },

    Form: {
      labelColor: brand.textMuted,
      verticalLabelPadding: '0 0 4px',
    },
  },
}
