import { theme } from 'antd'
import type { ThemeConfig } from 'antd'
import type { CSSProperties } from 'react'

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
    /*
     * 三档中性文字都是按"压在最深的浅底上仍要 ≥ 4.5:1"挑的，不是凭手感：
     *
     *        容器白   页面底   最深填充     改前（容器白）
     *   正文   15.44   14.27   13.74        15.44
     *   次级    6.44    5.95    5.73         4.55   ← 页面底上只有 4.20
     *   三级    5.23    4.84    4.66         2.55   ← 几乎融进背景
     *
     * 第三级不是"装饰性淡灰"：口径说明、卡片里的字段名、单证链上未生成的节点、
     * 未填的占位都是它，全是 12.5px 的正文，读不出来就等于没给。
     * 原来那两档在白底上勉强、在页面底（#f4f6fa）上已经不够 —— 而页面底正是
     * 口径说明所在的位置。三档压深之后梯度仍在（15.4 / 6.4 / 5.2），
     * 只是从"看不清的淡"变成"看得清的灰"。
     */
    textMuted: '#52606f',
    /** 第三级文字：占位、单位、口径说明。比 textMuted 更淡，但仍要能读。 */
    textFaint: '#626d7f',
    /*
     * 四档语义色：既是状态标签的字色，也是 Alert / Message / Progress 的
     * 提示色，所以每个值都按"压在自己的浅底上仍要 ≥ 4.5:1"挑。
     *
     * 原来的 #16a34a / #0ea5e9 / #d97706 是 tailwind 600 档，单看很亮眼，
     * 但配上 antd 从同一色相派生出来的底色（#d3e3d6 / #e6fbff / #fff7e6）
     * 只有 2.47 / 2.59 / 2.99 —— 状态标签是 12px 正文，这是"看不清"。
     * 压深一档（800 档）之后分别是 5.95 / 4.98 / 5.88，标签、提示条、
     * 图标同时受益；色相没变，认色仍然认得出来。
     */
    success: '#166534',
    warning: '#92400e',
    danger: '#b91c1c',
    /** 信息蓝，用于"进行中"这类中性状态，与主色区分开。 */
    info: '#0369a1',
    /*
     * 上面四档各自配一对"浅底 + 描边"：同一色相压进容器底 12% / 30%。
     *
     * 不写这一对就只能吃 antd 的派生值，而派生的底色深浅全看色相
     * （绿底 #d3e3d6 比红底 #fff2f0 深一大截，对比度也就跟着差一倍）。
     * 显式给出来之后四档的底色深浅一致，状态标签并排看才是同一个系列的。
     */
    successSoft: '#e3ede7',
    successBorder: '#b9d1c2',
    warningSoft: '#f2e8e2',
    warningBorder: '#dec6b7',
    dangerSoft: '#f7e4e4',
    dangerBorder: '#eabbbb',
    infoSoft: '#e1edf4',
    infoBorder: '#b3d2e3',
    fillSecondary: '#eef2f8',
    fillTertiary: '#f4f6fa',
    fillQuaternary: '#f8fafc',
    /*
     * 浮层（Tooltip）底色：明亮主题下用浅灰，与白色页面 / 白色卡片都拉开一档，
     * 一眼能认出"这是提示"而不是内容；文字仍用 text（深色），压在上面对比度很高。
     */
    tooltipBg: '#e9ebef',
    shadow: '0 6px 16px rgba(16, 24, 40, 0.08)',
    shadowSecondary: '0 6px 20px rgba(16, 24, 40, 0.10)',
    shadowCard: '0 1px 2px rgba(16, 24, 40, 0.05), 0 1px 3px rgba(16, 24, 40, 0.04)',
  },
  dark: {
    /*
     * 暗色主色是"夹"出来的，别只看一头去调它：
     * 白字要压在主色上（主按钮），主色又要当字用在深底上（链接、选中的菜单项、
     * 当前页码）。这两头互相打架 —— 主色越亮，按钮上的白字越差；越暗，当字用越差。
     *
     * 实机量到（种子 #4d8df7 经 darkAlgorithm 派生后是 #447BD5）：
     *     白字压主色 4.17 / 主色在页面底 4.43 / 主色在卡片底 4.11
     *
     * 三者同时过 4.5 在数学上无解：白字那一头要求主色亮度 ≤ 1.05/4.5−0.05
     * = 0.183，主色当字用在页面底（#0f141b）那一头要求 ≥ 4.5×0.0562−0.05
     * = 0.203。现在这版让按钮白字与页面底上的主色尽量贴着 4.5，
     * 代价是卡片底（#161c25）上的主色 4.11 —— 都还在 3:1 之上，能读。
     */
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
    /**
     * 第三级文字：占位、单位、口径说明。比 textMuted 更淡，但仍要能读。
     *
     * 暗色这一档是按对比度反推出来的，不是凭手感挑的：口径说明（页头副标题、
     * 「这页的数怎么算的」、卡片里的未填占位）都是 12.5px 的正文，
     * WCAG 对正文要求 4.5:1。原来的 #6d7b8d 在最亮的填充底（fillSecondary）
     * 上只有 3.44，浅一档的 #8493a5 是 4.74 —— 再淡就属于"看不清"而不是"次要"。
     */
    textFaint: '#8493a5',
    /*
     * 暗色这四档**不是**最终字色：darkAlgorithm 会按色板把它们再派生一次
     * （#2ecc71 → #2ab063、#f0a020 → #cf8b1e…），派生值比种子深一档。
     * 所以下面每对 soft / border 都是拿"派生后的字色"压进容器底 14% / 40%，
     * 不是拿种子压 —— 用种子算出来的底会浅一档，红档会掉到 3.8。
     */
    success: '#2ecc71',
    warning: '#f0a020',
    /**
     * 红档的种子比其它三档亮：派生后是 #dc7d6d，压 14% 的底上 4.77；
     * 沿用原来的 #f6705f 时派生出 #d46254，怎么压底都过不了 4.5
     * （连不带底、直接压在容器上才 4.62，稍一着底就掉下去）。
     */
    danger: '#ff8f7d',
    info: '#3ec2f0',
    successSoft: '#19312e',
    successBorder: '#1e573e',
    warningSoft: '#302c24',
    warningBorder: '#604822',
    dangerSoft: '#322a2f',
    dangerBorder: '#654342',
    infoSoft: '#1b303d',
    infoBorder: '#245469',
    fillSecondary: '#202834',
    fillTertiary: '#1a212b',
    fillQuaternary: '#151b23',
    /*
     * 浮层底色（暗色）：比页面背景（#0f141b）再压深一档的深灰，浮在内容之上能
     * 立刻认出来；文字用 text（浅色），压在上面对比度很高、长时间读不累。
     */
    tooltipBg: '#0b0e14',
    shadow: '0 6px 16px rgba(0, 0, 0, 0.45)',
    shadowSecondary: '0 8px 24px rgba(0, 0, 0, 0.5)',
    shadowCard: '0 1px 2px rgba(0, 0, 0, 0.4)',
  },
} as const

/** 亮色语义色板，index.css 与旧引用仍按这个名字取色。 */
export const brand = palette.light

/** 全站统一圆角。表格、卡片、按钮、输入框共用一套档位，避免圆角混搭。 */
const RADIUS = 8

/**
 * 外壳几何：顶栏、侧栏、内容内距与标签条的度量。
 *
 * 这些值此前分居三处：AppLayout 的模块常量（侧栏两档宽度、抽屉宽度）、
 * index.css 的字面量（内距、品牌块、标签胶囊）、以及内联的 `--app-header-height`。
 * 改一个侧栏宽度要在两个文件各改一次，漏一处就是一条错位的缝 ——
 * 顶栏与品牌块必须落在同一条水平线上，正是最典型的一处。
 *
 * 这里只管"值放在哪"，不管"值是多少"：换皮时对照参考站实测改这一处。
 * 形态是 **px 数值**（不是字符串），因为侧栏宽度要交给 antd 的 `Layout.Sider`；
 * CSS 侧通过 `shellCssVars` 拿到同一批值的 `--shell-*` 变量。
 *
 * 每个变量在 index.css 里都带一个同值兜底：`.brand` 在登录页也用到，
 * 那里没有外壳元素，取不到变量。
 */
export const shell = {
  /**
   * 顶栏高度。顶栏与侧栏品牌块共用：分处两列、却必须落在同一条水平线上。
   *
   * 以下四个尺寸取自 Vben 5.7 的 `defaultPreferences`
   * （`sidebar.width` 224 / `sidebar.collapseWidth` 60 / `header.height` 50）。
   * 演示站当前对无头浏览器全部返回 503（`/js/*` 无一加载成功），截不到实测图，
   * 所以基线用源码默认值；页面上量到差异时改这一处，不要改 CSS。
   */
  headerHeight: 50,
  /** 顶栏左右内距，与品牌块的内距分开：后者收窄后要居中。 */
  headerPaddingInline: 8,
  /** 顶栏内元素间距（标签条与右侧待办 / 头像之间）。 */
  headerGap: 8,

  /**
   * 顶栏下方那条标签条的高度。
   *
   * 标签从顶栏里搬出来之后单独占一条：顶栏放面包屑与全局操作，
   * 标签条放"这半天开着哪些页"，两者的信息密度不一样，挤在一行里会互相让位。
   */
  tabbarRowHeight: 40,

  siderWidth: 224,
  /*
   * 收起宽度 60（Vben 的 60，不是 antd 默认的 80）。
   *
   * antd 的内联折叠菜单宽度是 `controlHeightLG * 2`（≈80），不跟着 Sider 走 ——
   * 只改 Sider 会让菜单比侧栏宽一截，图标被裁。所以 Menu 的 `collapsedWidth`
   * 必须同时给成这个值（见下面两处 Menu token）。
   */
  siderCollapsedWidth: 60,
  /** 抽屉与侧栏同宽：Vben 没有为移动端单列一档宽度。 */
  mobileSiderWidth: 224,
  siderBrandPaddingInline: 16,
  /** 底部折叠开关：不参与滚动，钉在侧栏底部。 */
  siderTriggerHeight: 40,

  /** 内容内距：宽屏 24，窄屏 12（与 refine 的 ThemedLayout 同档）。 */
  tabPanePadding: 24,
  tabPanePaddingNarrow: 12,

  /** 品牌标志块：登录页会按自己的尺寸覆写，所以变量只在侧栏里有意义。 */
  brandMarkSize: 28,
  brandMarkRadius: 8,
  brandMarkFontSize: 15,
  brandGap: 10,

  /** 顶栏里的标签胶囊。 */
  tabbar: {
    // 字号与 Vben 的 text-sm 一致（14）。
    // paddingInline 同时是"左右间隙"：标签文字居中后，右侧这一份空出来给钉标 /
    // 关闭按钮（两者钉在同一位置，见 index.css），它们显隐不会挤动文字。
    paddingBlock: 5,
    paddingInline: 22,
    radius: 7,
    gap: 6,
    fontSize: 14,
  },

  /** 顶栏右侧的用户块。 */
  user: {
    gap: 8,
    paddingInline: 8,
    fontSize: 13.5,
  },

  /** 顶部加载进度条：2px 的线，不占布局。 */
  progressHeight: 2,
} as const

/**
 * 把外壳几何下发成 CSS 变量。
 *
 * 挂在 `.app-shell` 上（见 AppLayout），index.css 的结构性样式引用变量、
 * 不再抄数字。仍保留字面兜底值：登录页在外壳之外，只有 `.brand` 这类共用样式。
 */
export const shellCssVars = {
  '--shell-header-height': `${shell.headerHeight}px`,
  '--shell-header-gap': `${shell.headerGap}px`,
  '--shell-header-padding-inline': `${shell.headerPaddingInline}px`,
  '--shell-tabbar-row-height': `${shell.tabbarRowHeight}px`,
  '--shell-tab-pane-padding': `${shell.tabPanePadding}px`,
  '--shell-tab-pane-padding-narrow': `${shell.tabPanePaddingNarrow}px`,
  '--shell-sider-brand-padding': `${shell.siderBrandPaddingInline}px`,
  '--shell-sider-trigger-height': `${shell.siderTriggerHeight}px`,
  '--shell-brand-mark-size': `${shell.brandMarkSize}px`,
  '--shell-brand-mark-radius': `${shell.brandMarkRadius}px`,
  '--shell-brand-mark-font-size': `${shell.brandMarkFontSize}px`,
  '--shell-brand-gap': `${shell.brandGap}px`,
  '--shell-tab-padding-block': `${shell.tabbar.paddingBlock}px`,
  '--shell-tab-padding-inline': `${shell.tabbar.paddingInline}px`,
  '--shell-tab-radius': `${shell.tabbar.radius}px`,
  '--shell-tab-gap': `${shell.tabbar.gap}px`,
  '--shell-tab-font-size': `${shell.tabbar.fontSize}px`,
  '--shell-user-gap': `${shell.user.gap}px`,
  '--shell-user-padding-inline': `${shell.user.paddingInline}px`,
  '--shell-user-font-size': `${shell.user.fontSize}px`,
  '--shell-progress-height': `${shell.progressHeight}px`,
} as CSSProperties

/**
 * 表格与卡片的行高/内距，密度切换时按比例收一档。
 *
 * `formItemGap` 是表单项下边距的**唯一来源**：CSS 里那条
 * `.entity-form .ant-form-item { margin-bottom }` 已经删了 ——
 * 同样是 20/14 的两份写法，改一处就会露成"控件小了、留白还是大的"。
 * 结构性间距（页头、筛选条、卡片内距）不在 antd 的 token 里，
 * 由 index.css 的 --gap-* / --pad-block 一组变量按同一个 [data-density] 收档。
 */
const density = (compact: boolean) => ({
  cellPaddingBlock: compact ? 8 : 12,
  cardPadding: compact ? 14 : 20,
  headerHeight: compact ? 46 : 50,
  formItemGap: compact ? 14 : 20,
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
      /*
       * 悬停色也要显式给：antd 派生的是**比主色更浅**的一档（默认主色
       * #1677ff → 悬停 #4096ff），浅一档的蓝压在主色淡底上只有 3.46，
       * 而强调标记（「默认地址 / 本币 / 主联系人」）正是这个组合。
       * 色板里的 primaryHover 在亮色是压深一档、暗色是提亮一档，正好够。
       */
      colorPrimaryHover: c.primaryHover,
      /*
       * 主色淡底。antd 派生的那个在暗色下是 #253858（配主色只有 2.82），
       * 而 primarySoft 在暗色是带透明度的主色，压在容器底上 5.48 ——
       * 与 Menu 选中项、Select 选中项、表格行悬停同源，别再派生第二份。
       */
      colorPrimaryBg: c.primarySoft,
      colorSuccess: c.success,
      colorWarning: c.warning,
      colorError: c.danger,
      colorInfo: c.info,
      /*
       * 状态标签（Tag 的 success / processing / warning / error）取的就是
       * colorSuccess + colorSuccessBg + colorSuccessBorder 这一组，
       * 所以底与边必须显式给：吃派生值的话四档深浅不齐（绿底比红底深一大截），
       * 对比度也就从 2.47 到 4.42 各不相同。Alert / Message / Progress 的
       * 提示色用的是同一组，一起受益。
       */
      colorSuccessBg: c.successSoft,
      colorSuccessBorder: c.successBorder,
      colorWarningBg: c.warningSoft,
      colorWarningBorder: c.warningBorder,
      colorErrorBg: c.dangerSoft,
      colorErrorBorder: c.dangerBorder,
      colorInfoBg: c.infoSoft,
      colorInfoBorder: c.infoBorder,
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
      /*
       * 不给 Layout 组件 token：外壳没有 <Layout> 根节点（.app-shell 是普通 div，
       * Header / Sider 各自单独渲染），而开 cssVar 时这批变量注册在
       * `.css-var-*.ant-layout` 复合类上 —— DOM 里没有元素同时带这两个类，
       * token 全部落空。headerPadding / headerBg 曾因此静默失效：
       * 顶栏内距变成 0，面包屑贴着侧栏分界线，看着像被裁掉一截。
       * 顶栏几何改走 --shell-*，顶栏底色直接写在 index.css 里。
       * 侧栏的 Layout token 在 buildSiderThemeConfig 里：那一层关了 cssVar，
       * 用字面值，不受此影响。
       */

      /*
       * 菜单的通用配色（页面里的菜单，不是侧栏）。
       *
       * 侧栏有自己的 ConfigProvider，读的是 buildSiderThemeConfig 里的那一份 ——
       * 这里改了不会影响侧栏，反过来也一样。这正是"两层主题分离"的意思。
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
        // 圆角与上下间距照 Vben 的 menu-item-radius / margin-y：8px 与 2px。
        itemMarginInline: 8,
        itemMarginBlock: 2,
        itemBorderRadius: RADIUS,
        subMenuItemBorderRadius: RADIUS,
        // 折叠态宽度跟随侧栏（antd 默认 80，比 60 的侧栏宽，图标会被裁）。
        collapsedWidth: shell.siderCollapsedWidth,
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

      /*
       * 标签：中性底 + 正文字色。
       *
       * 之前字色用的是 textMuted（#6b7788 在这个灰底上只有 4.05），
       * 而"草稿 / 已完成 / 停用"这些中性标签恰恰是读得最多的几个；
       * 底是灰的已经足够弱化了，字色再压淡就只是糊。
       * 圆角随密度收一档：紧凑模式下整屏标签更多，6px 会显得圆胖。
       */
      Tag: {
        defaultBg: c.fillSecondary,
        defaultColor: c.text,
        borderRadiusSM: compact ? 4 : 6,
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
        /*
         * 浮层跟着主题走，且底色与主题色明显区分：
         * 明亮用浅灰（c.tooltipBg）、深色用略深于背景的深灰（同样取 c.tooltipBg），
         * 文字都用 c.text（亮色深字 / 暗色浅字），对比度都拉得很够。
         *
         * colorBgSpotlight 是 tooltip 底色，colorTextLightSolid 是它上面的字色
         * （antd 内部是 tooltipBg / tooltipColor）。
         */
        colorBgSpotlight: c.tooltipBg,
        colorTextLightSolid: c.text,
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

/* ---------- 侧栏主题：与页面主题分离 ---------- */

/** 侧栏用哪套配色：follow 跟随页面明暗，light / dark 单独指定。 */
export type SiderMode = 'follow' | 'light' | 'dark'

/** 偏好 → 实际色板。只有 follow 才去看页面当前是亮还是暗。 */
export function resolveSiderMode(siderMode: SiderMode, pageMode: ColorMode): ColorMode {
  return siderMode === 'follow' ? pageMode : siderMode
}

/**
 * 侧栏的 CSS 变量。
 *
 * 侧栏不再是"页面主题染出来的一块"，而是自己的一套配色。但 antd 的 cssVar
 * 变量是**全局**的一套值，侧栏里的结构性样式（品牌块、分隔线、滚动条）
 * 若引用 --ant-*，切侧栏配色时它们不会跟着变 —— 深色侧栏上会留一条浅色分隔线。
 * 所以侧栏内部只认 --sider-*，值由这里按当前侧栏色板给出。
 */
export function siderCssVars(mode: ColorMode): CSSProperties {
  const c = palette[mode]
  return {
    '--sider-bg': c.surface,
    '--sider-border': c.border,
    '--sider-text': c.text,
    '--sider-text-muted': c.textMuted,
    '--sider-text-faint': c.textFaint,
    '--sider-hover-bg': mode === 'dark' ? c.fillSecondary : c.fillTertiary,
    // 品牌标志的渐变也要跟着侧栏自己的主色走：深色侧栏配浅色页面的主色时，
    // 那块小方块上的深色字母会掉到 3.4:1 以下（见 .brand-mark 的注释）。
    '--sider-primary': c.primary,
    '--sider-primary-hover': c.primaryHover,
    '--sider-scroll-thumb': `color-mix(in srgb, ${c.text} 18%, transparent)`,
  } as CSSProperties
}

/**
 * 侧栏主题：套在侧栏子树外面的第二个 ConfigProvider。
 *
 * 三条都是踩过才写下来的：
 *
 * 1. **关掉 cssVar**。打开时 antd 把 token 以类名形式注册成全局样式
 *    （`ant-menu-css-var` 这类），两层主题撞在同一批类名上、后注册的赢 ——
 *    侧栏一换色内容区跟着变，"独立"就只剩开关的样子。关掉之后侧栏里的组件
 *    用字面色值，与外层的 --ant-* 互不干扰。
 * 2. **显式给 algorithm**。嵌套 ConfigProvider 的 token 是从 defaultSeedToken
 *    重算的，不给暗色算法，深色侧栏的派生色（描边、填充、悬停底）会是一套浅色值。
 * 3. **组件级 token 重给一遍**。Menu 的 itemColor / itemSelectedBg 属于页面主题，
 *    深色侧栏照抄过去就是深底深字。
 */
export function buildSiderThemeConfig({
  mode = 'light',
  compact = false,
}: {
  mode?: ColorMode
  compact?: boolean
} = {}): ThemeConfig {
  const c = palette[mode]

  return {
    cssVar: false,
    algorithm: [
      mode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
      ...(compact ? [theme.compactAlgorithm] : []),
    ],

    token: {
      colorPrimary: c.primary,
      colorPrimaryHover: c.primaryHover,
      colorPrimaryBg: c.primarySoft,
      colorBgLayout: c.surface,
      colorBgContainer: c.surface,
      colorBgElevated: c.surfaceElevated,
      colorBorder: c.border,
      colorBorderSecondary: c.border,
      colorText: c.text,
      colorTextSecondary: c.textMuted,
      colorTextTertiary: c.textFaint,
      colorTextQuaternary: c.textFaint,
      colorFillSecondary: c.fillSecondary,
      colorFillTertiary: c.fillTertiary,
      colorFillQuaternary: c.fillQuaternary,
      borderRadius: RADIUS,
      borderRadiusLG: RADIUS + 2,
      borderRadiusSM: RADIUS - 2,
      fontSize: 14,
      sizeStep: 4,
      sizeUnit: 4,
      controlHeight: 34,
      // 折叠动画与侧栏宽度过渡同一个节奏，展开时不会先慢后快。
      motionDurationFast: '0.1s',
      motionDurationMid: '0.18s',
      motionDurationSlow: '0.26s',
    },

    components: {
      Layout: {
        // 侧栏底色只由这一个 token 决定：antd 的 -dark / -light 修饰类
        // 现在只影响触发器，不参与背景，所以不必给 Sider 传 theme。
        siderBg: c.surface,
      },

      Menu: {
        itemBg: 'transparent',
        subMenuItemBg: 'transparent',
        itemColor: c.textMuted,
        itemHoverColor: c.text,
        // 深色下 fillTertiary 与侧栏底色几乎同值，悬停看不出来，往上取一档。
        itemHoverBg: mode === 'dark' ? c.fillSecondary : c.fillTertiary,
        itemSelectedBg: c.primarySoft,
        itemSelectedColor: c.primary,
        itemActiveBg: c.primarySoft,
        groupTitleColor: c.textFaint,
        groupTitleFontSize: 12.5,
        itemHeight: compact ? 34 : 38,
        itemMarginInline: 8,
        itemMarginBlock: 2,
        itemBorderRadius: RADIUS,
        subMenuItemBorderRadius: RADIUS,
        // 侧栏有独立的 ConfigProvider，这一份必须自己再给一遍。
        collapsedWidth: shell.siderCollapsedWidth,
      },
    },
  }
}

/** 亮色 + 标准密度的默认主题：登录页等挂不到偏好 Provider 之外的地方直接用。 */
export const themeConfig = buildThemeConfig()
