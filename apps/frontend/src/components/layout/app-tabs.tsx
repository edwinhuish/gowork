import { Tabs } from 'antd'

import { HOME_TAB_KEY, useTabs } from '@/providers/tabs'

/**
 * 标签条：顶栏与内容区之间那一条。
 *
 * 用 antd 的 editable-card 而不是自绘：标签多了之后它自带横向滚动与「更多」下拉，
 * 窄屏（一屏放不下两个标签）就不用手写一套溢出逻辑。
 *
 * 这里只负责"看起来怎么样"和"点了要发生什么"：
 * 点击与关闭都交给 TabsProvider —— 切换标签页的实质是把地址栏导航过去，
 * 由地址栏再反过来激活标签页，只有一份真相（见 providers/tabs.tsx）。
 */
export function AppTabs() {
  const { tabs, activeKey, activate, close } = useTabs()

  return (
    <div className="app-tabbar">
      <Tabs
        type="editable-card"
        size="small"
        hideAdd
        activeKey={activeKey}
        items={tabs.map((tab) => ({
          key: tab.key,
          // title 兜住被省略号截断的长标题：单据详情这类标题经常放不下。
          label: <span title={tab.label}>{tab.label}</span>,
          // 首页固定：它是所有标签页关完之后唯一的落点。
          closable: tab.key !== HOME_TAB_KEY,
        }))}
        onChange={activate}
        onEdit={(targetKey, action) => {
          if (action === 'remove' && typeof targetKey === 'string') close(targetKey)
        }}
      />
    </div>
  )
}
