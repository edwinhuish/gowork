import { Tooltip } from '@/components/ui/Tooltip'
import {
  CloseOutlined,
  FullscreenExitOutlined,
  FullscreenOutlined,
  MoreOutlined,
  PushpinOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { Button, Dropdown, Tabs } from 'antd'
import type { MenuProps } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { DragEvent as ReactDragEvent } from 'react'

import { isFixedTab, useTabs } from '@/providers/tabs'

/**
 * 标签条：现在住在顶栏里（AppLayout 的 AppHeader 把它夹在左侧与右侧按钮区之间）。
 *
 * 用 antd 的 editable-card 而不是自绘：标签多了之后它自带横向滚动与「更多」下拉，
 * 窄屏（一屏放不下两个标签）就不用手写一套溢出逻辑。
 * 它自己不再画底边与底色 —— 那条边界是顶栏的 border，往外一层已经裁掉溢出。
 *
 * 这里只负责"看起来怎么样"和"点了要发生什么"：
 * 点击与关闭都交给 TabsProvider —— 切换标签页的实质是把地址栏导航过去，
 * 由地址栏再反过来激活标签页，只有一份真相（见 providers/tabs.tsx）。
 *
 * 拖拽排序走浏览器原生的 draggable，没有引第三方拖拽库：
 * 按住标签横向拖 → 被拖的那一项原地留空、途经的标签各让出一格（空位跟着指针走到落点）
 * → 松手后由 TabsProvider.move 改顺序。
 * 拖拽只动"排列顺序"，不动地址栏，所以和切换、激活态不冲突：
 * 当前页仍然是地址栏那一个，把别的标签拖到前面也不会把它变成当前页。
 * 触屏上 HTML5 拖拽本来就不触发，标签条在那边保持"点一下切换"的老样子。
 */

/** 落点：落在目标标签的左半边就插到它前面，右半边插到它后面。 */
interface DropTarget {
  key: string
  after: boolean
}

const TAB_SELECTOR = '.ant-tabs-tab'

/**
 * 从事件目标回溯到它所在的那个标签节点。
 *
 * 拖拽必须知道"鼠标现在压在哪一个标签上"，而 antd 的 items 只接受 label / closable，
 * 不给 className，只能认它渲染出来的结构：标签是 `.ant-tabs-tab`，key 写在
 * `data-node-key` 上（rc-tabs 的 TabNode；它只转义双引号，pathname 里不会出现）。
 */
function tabNodeOf(target: EventTarget | null): HTMLElement | null {
  if (!(target instanceof Element)) return null
  return target.closest<HTMLElement>(TAB_SELECTOR)
}

/**
 * 一个标签占的横向空间：自身宽度 + 它和邻居之间的缝。
 *
 * 缝不在任何可读的样式里（antd 给 2px、本项目改成 6px），从相邻两个标签的位置差
 * 推出来最省事；标签宽度各不相同，所以不能拿"下一个的起点减这一个的起点"直接当格宽。
 */
function slotWidth(list: HTMLElement[], from: number): number {
  const node = list[from]
  if (!node) return 0
  const prev = list[from - 1]
  const next = list[from + 1]
  const gap = next
    ? next.offsetLeft - node.offsetLeft - node.offsetWidth
    : prev
      ? node.offsetLeft - prev.offsetLeft - prev.offsetWidth
      : 0
  return node.offsetWidth + gap
}

/**
 * 闪避位移：被拖的那一项原地留空，途经的标签整体让出一格，"空位"于是跟着指针走到落点。
 *
 * 落点只有两种（插到 target 之前 / 之后），所以横向挪一格就够：往前拖时
 * (from, target] 里的标签左移一格，往后拖时 [target, from) 里的标签右移一格。
 * 返回 0 的标签站在原地不动。
 */
function dodgeShift(
  index: number,
  from: number,
  target: number,
  after: boolean,
  slot: number,
): number {
  if (from === -1 || target === -1 || index === from) return 0
  if (target > from) {
    const inRange = after ? index > from && index <= target : index > from && index < target
    return inRange ? -slot : 0
  }
  if (target < from) {
    const inRange = after ? index > target && index < from : index >= target && index < from
    return inRange ? slot : 0
  }
  return 0
}

interface AppTabsProps {
  /** 是否处于全屏模式（侧栏与顶栏收起，只留标签条与内容）。 */
  fullscreen: boolean
  /** 切换全屏 / 退出全屏。 */
  onToggleFullscreen: () => void
}

export function AppTabs({ fullscreen, onToggleFullscreen }: AppTabsProps) {
  const { tabs, activeKey, activate, close, refresh, closeOthers, closeLeft, closeRight, closeAll, move } =
    useTabs()

  const barRef = useRef<HTMLDivElement>(null)
  /** 正在被拖的标签，也是"这一次拖拽有没有意义"的开关。 */
  const [dragKey, setDragKey] = useState<string | null>(null)
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null)

  /*
   * 右键菜单：作用对象是"被右键的那一个标签"，不是当前激活的标签。
   *
   * 在 `contextmenu` 的**捕获阶段**记下目标键，菜单随后由 antd 的 Dropdown
   * 按指针位置弹出 —— 菜单项要显示"关闭左侧（3）"这类计数，必须先知道是谁。
   * 右键落在空白处时记 null，退化成对当前页操作。
   */
  const [contextKey, setContextKey] = useState<string | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)

  /*
   * 拖拽要用的属性、类与位移只能打在 antd 渲染出来的标签节点上（同上：items 没有入口）。
   *   draggable      整块标签都能按住拖；只有文字那一段能拖的话，从内距落手就拖不动
   *   .is-dragging   被拖的那一项原地留空（样式见 index.css）
   *   transform      其余标签的闪避位移
   *
   * 用位移而不是"边拖边改顺序"：DOM 顺序一变，antd 的横向滚动与「更多」下拉都要重算，
   * 正在进行的这次原生拖拽也可能被浏览器中断。位移只是画面上挪一格，松手才真正改数据。
   */
  useEffect(() => {
    const list = Array.from(barRef.current?.querySelectorAll<HTMLElement>(TAB_SELECTOR) ?? [])
    const from = list.findIndex((node) => node.dataset.nodeKey === dragKey)
    const target = list.findIndex((node) => node.dataset.nodeKey === dropTarget?.key)
    const after = dropTarget?.after ?? false
    const slot = slotWidth(list, from)

    list.forEach((node, index) => {
      node.draggable = true
      node.classList.toggle('is-dragging', index === from)
      const shift = dodgeShift(index, from, target, after, slot)
      node.style.transform = shift === 0 ? '' : `translateX(${shift}px)`
    })
  }, [tabs, dragKey, dropTarget])

  const resetDrag = () => {
    setDragKey(null)
    setDropTarget(null)
  }

  const handleDragStart = (event: ReactDragEvent<HTMLDivElement>) => {
    const key = tabNodeOf(event.target)?.dataset.nodeKey
    if (!key) return
    // Firefox 不往 dataTransfer 里写点东西就不会真的开始拖。
    event.dataTransfer.setData('text/plain', key)
    event.dataTransfer.effectAllowed = 'move'
    setDragKey(key)
  }

  const handleDragOver = (event: ReactDragEvent<HTMLDivElement>) => {
    if (!dragKey) return
    // 不 preventDefault 就不允许放下（光标带禁止符），也就收不到 drop。
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'

    const node = tabNodeOf(event.target)
    // 指在标签之间的缝里：保持上一个落点。缝只有几像素，清掉会让空位来回跳。
    if (!node) return

    const key = node.dataset.nodeKey
    // 压在自己身上：没有可落的位置，收掉落点，此时松手即取消。
    if (!key || key === dragKey) {
      setDropTarget((prev) => (prev === null ? prev : null))
      return
    }

    // 用当前（可能已经被位移过的）可见位置判断左右半边：用户看到哪半边就是哪半边。
    const rect = node.getBoundingClientRect()
    const after = event.clientX > rect.left + rect.width / 2
    setDropTarget((prev) =>
      prev && prev.key === key && prev.after === after ? prev : { key, after },
    )
  }

  const handleDragLeave = (event: ReactDragEvent<HTMLDivElement>) => {
    // 在标签之间穿行时 relatedTarget 还在条内，只有真的移出整条才收落点。
    const next = event.relatedTarget
    if (next instanceof Node && event.currentTarget.contains(next)) return
    setDropTarget((prev) => (prev === null ? prev : null))
  }

  const handleDrop = (event: ReactDragEvent<HTMLDivElement>) => {
    event.preventDefault()
    if (dragKey && dropTarget) move(dragKey, dropTarget.key, dropTarget.after)
    resetDrag()
  }

  /*
   * 右键菜单的作用对象：被右键的标签；落在空白处时退化成当前页。
   * 计数用于禁用 —— "关闭左侧"在一路都是固定标签时应该是灰的，而不是点了没反应。
   */
  const menuTarget = contextKey ?? activeKey
  const targetIndex = tabs.findIndex((tab) => tab.key === menuTarget)
  const closableIn = (list: typeof tabs) => list.filter((tab) => !isFixedTab(tab.key)).length

  const menuItems: MenuProps['items'] = [
    { key: 'refresh', icon: <ReloadOutlined />, label: '刷新' },
    { type: 'divider' },
    { key: 'close', icon: <CloseOutlined />, label: '关闭', disabled: isFixedTab(menuTarget) },
    {
      key: 'close-others',
      label: '关闭其他',
      disabled: closableIn(tabs.filter((tab) => tab.key !== menuTarget)) === 0,
    },
    {
      key: 'close-left',
      label: '关闭左侧',
      disabled: targetIndex <= 0 || closableIn(tabs.slice(0, targetIndex)) === 0,
    },
    {
      key: 'close-right',
      label: '关闭右侧',
      disabled: targetIndex === -1 || closableIn(tabs.slice(targetIndex + 1)) === 0,
    },
    { type: 'divider' },
    { key: 'close-all', label: '全部关闭', disabled: closableIn(tabs) === 0 },
  ]

  /*
   * 右侧工具组：与 Vben 一样是「批量操作 + 刷新」两件。
   *
   * 批量操作永远作用于**当前页**（不是"最后右键的那一个"）：工具组在条尾，
   * 没有"点的是谁"这层信息，硬塞一个作用对象只会让人猜。
   */
  const toolIndex = tabs.findIndex((tab) => tab.key === activeKey)
  const toolItems: MenuProps['items'] = [
    {
      key: 'close',
      label: '关闭当前',
      disabled: isFixedTab(activeKey) || tabs.length <= 1,
    },
    {
      key: 'close-others',
      label: '关闭其他',
      disabled: closableIn(tabs.filter((tab) => tab.key !== activeKey)) === 0,
    },
    {
      key: 'close-left',
      label: '关闭左侧',
      disabled: toolIndex <= 0 || closableIn(tabs.slice(0, toolIndex)) === 0,
    },
    {
      key: 'close-right',
      label: '关闭右侧',
      disabled: toolIndex === -1 || closableIn(tabs.slice(toolIndex + 1)) === 0,
    },
    { type: 'divider' },
    { key: 'close-all', label: '全部关闭', disabled: closableIn(tabs) === 0 },
  ]

  const handleToolClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'close') close(activeKey)
    else if (key === 'close-others') closeOthers(activeKey)
    else if (key === 'close-left') closeLeft(activeKey)
    else if (key === 'close-right') closeRight(activeKey)
    else if (key === 'close-all') closeAll()
  }

  const handleMenuClick: MenuProps['onClick'] = ({ key }) => {
    setMenuOpen(false)
    if (key === 'refresh') refresh(menuTarget)
    else if (key === 'close') close(menuTarget)
    else if (key === 'close-others') closeOthers(menuTarget)
    else if (key === 'close-left') closeLeft(menuTarget)
    else if (key === 'close-right') closeRight(menuTarget)
    else if (key === 'close-all') closeAll()
  }

  return (
    <div className="app-tabbar-row">
      <Dropdown
        trigger={['contextMenu']}
        menu={{ items: menuItems, onClick: handleMenuClick }}
        open={menuOpen}
        onOpenChange={setMenuOpen}
      >
        <div
          ref={barRef}
          className="app-tabbar"
          // 捕获阶段先记下"点了哪一个标签"，菜单项随后才由 Dropdown 按指针位置弹出。
          onContextMenuCapture={(event) => {
            setContextKey(tabNodeOf(event.target)?.dataset.nodeKey ?? null)
          }}
          onDragStart={handleDragStart}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          // 拖到条外松手、或者按 Esc 取消时，浏览器只给 dragend，落点状态在这里收。
          onDragEnd={resetDrag}
        >
          <Tabs
            type="editable-card"
            size="small"
            hideAdd
            activeKey={activeKey}
            items={tabs.map((tab) => ({
              key: tab.key,
              label: (
                // title 兜住被省略号截断的长标题：单据详情这类标题经常放不下。
                <span className="app-tab-label" title={tab.label}>
                  {/* 固定标签带钉标：它关不掉，这件事要看得出来。 */}
                  {isFixedTab(tab.key) && <PushpinOutlined className="app-tab-pin" />}
                  {tab.label}
                </span>
              ),
              // 首页固定：它是所有标签页关完之后唯一的落点。
              closable: !isFixedTab(tab.key),
            }))}
            onChange={activate}
            onEdit={(targetKey, action) => {
              if (action === 'remove' && typeof targetKey === 'string') close(targetKey)
            }}
          />
        </div>
      </Dropdown>

      {/* 与 Vben 的工具栏一致：批量操作（下拉）+ 刷新（单独一个，最常用）。 */}
      <div className="app-tabbar-tools">
        <Dropdown trigger={['click']} menu={{ items: toolItems, onClick: handleToolClick }}>
          <Button type="text" size="small" aria-label="批量操作" icon={<MoreOutlined />} />
        </Dropdown>
        {/* 这两个按钮在最右侧：tooltip 用 bottomRight，让浮层向右对齐、向左展开，
            自然离右缘留出间距，不会被夹到视口右边缘（居中放置不会水平翻转，会贴边）。 */}
        <Tooltip title="刷新当前页" placement="bottomRight">
          <Button
            type="text"
            size="small"
            aria-label="刷新当前页"
            icon={<ReloadOutlined />}
            onClick={() => refresh(activeKey)}
          />
        </Tooltip>
        {/* 全屏切换：最右侧。进入后侧栏与顶栏自动收起，只留标签条与内容（见 index.css 的 .is-fullscreen）。 */}
        <Tooltip title={fullscreen ? '退出全屏' : '进入全屏'} placement="bottomRight">
          <Button
            type="text"
            size="small"
            aria-label={fullscreen ? '退出全屏' : '进入全屏'}
            icon={fullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
            onClick={onToggleFullscreen}
          />
        </Tooltip>
      </div>
    </div>
  )
}
