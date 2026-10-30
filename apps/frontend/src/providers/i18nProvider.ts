import type { I18nProvider } from '@refinedev/core'

/**
 * refine 的中文文案。
 *
 * antd 由 `ConfigProvider locale={zhCN}` 管，但 refine 自己的按钮与面包屑
 * 走它自己的 i18n：没有 i18nProvider 时，`Save` / `Create` / `Delete` 会以英文
 * 混在中文界面里（表单页的保存按钮和面包屑最后一格就是）。
 *
 * 只翻 refine 真正会用到的键：资源名来自 App.tsx 的 resources.label，
 * 业务文案来自各页面，这里不碰。没命中的键回落到 refine 传进来的默认值，
 * 宁可显示英文也不要显示一个裸 key。
 */
const MESSAGES: Record<string, string> = {
  'actions.create': '新建',
  'actions.edit': '编辑',
  'actions.show': '详情',
  'actions.delete': '删除',
  'actions.list': '列表',
  'actions.clone': '复制',
  'actions.save': '保存',
  'actions.cancel': '取消',
  'actions.refresh': '刷新',
  'actions.add': '添加',

  'buttons.create': '新建',
  'buttons.edit': '编辑',
  'buttons.show': '查看',
  'buttons.delete': '删除',
  'buttons.save': '保存',
  'buttons.cancel': '取消',
  'buttons.refresh': '刷新',
  'buttons.import': '导入',
  'buttons.export': '导出',
  'buttons.clone': '复制',
  'buttons.logout': '退出登录',
  'buttons.add': '添加',
  'buttons.remove': '移除',
  'buttons.list': '列表',
  'buttons.search': '搜索',

  'notifications.success': '操作成功',
  'notifications.error': '操作失败',
  'notifications.createSuccess': '创建成功',
  'notifications.editSuccess': '保存成功',
  'notifications.deleteSuccess': '删除成功',

  'pages.create.title': '新建',
  'pages.edit.title': '编辑',
  'pages.show.title': '详情',
  'pages.list.title': '列表',
  'pages.error.404': '页面不存在',
  'pages.error.info': '页面出错了',
  'pages.error.backHome': '返回首页',
  'pages.login.title': '登录',
  'pages.login.signin': '登录',
  'pages.login.username': '用户名',
  'pages.login.password': '密码',
  'pages.login.rememberMe': '记住我',
  'pages.login.forgotPassword': '忘记密码',
  'pages.login.success': '登录成功',
  'pages.login.error': '用户名或密码不正确',
}

export const i18nProvider: I18nProvider = {
  translate: (key: string, options?: Record<string, unknown>, defaultMessage?: string) => {
    // refine 有两种调用姿势：`translate(key, options, defaultMessage)` 和
    // `translate(key, defaultMessage)`（safeTranslate 与列表标题都用后者）。
    // 第二参是字符串时它就是默认文案 —— 当成 options 会丢掉资源名，
    // 标题会变成 products.titles.list 这种裸 key。
    const fallback = typeof options === 'string' ? options : defaultMessage
    const message = MESSAGES[key]
    if (!message) return fallback ?? key
    if (!options || typeof options === 'string') return message
    return message.replace(/\{\{\s*(\w+)\s*\}\}/g, (_, name: string) =>
      String(options[name] ?? ''),
    )
  },
  changeLocale: () => Promise.resolve(),
  getLocale: () => 'zh-CN',
}
