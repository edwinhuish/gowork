import { SearchOutlined } from '@ant-design/icons'
import { useMenu, type TreeMenuItem } from '@refinedev/core'
import { AutoComplete, Input } from 'antd'
import type { InputRef } from 'antd'
import { useEffect, useMemo, useRef } from 'react'
import { useNavigate } from 'react-router'

/**
 * 顶栏的模块搜索。
 *
 * 30 个模块分在 7 个组里，找"条款库"这种半年用一次的页面要翻两层。
 * 这里直接把 useMenu 的菜单树拍平成"分组 · 模块"的下拉：不另维护一份模块清单，
 * 新加的资源自动出现。
 *
 * 快捷键 `/` 聚焦 —— 只在焦点不在输入框里时生效，否则表单里打斜杠会被吞掉。
 */
type ModuleOption = { value: string; label: string }

function flatten(items: TreeMenuItem[], parent?: string): ModuleOption[] {
  return items.flatMap((item) => {
    if (item.children.length > 0) {
      return flatten(item.children, item.label ?? item.name)
    }
    if (!item.route) return []
    const name = item.label ?? item.name
    return [{ value: item.route, label: parent ? `${parent} · ${name}` : name }]
  })
}

export function ModuleSearch() {
  const { menuItems } = useMenu()
  const navigate = useNavigate()
  const inputRef = useRef<InputRef>(null)

  const options = useMemo(() => flatten(menuItems), [menuItems])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      const el = document.activeElement
      const tag = el?.tagName?.toLowerCase()
      if (tag === 'input' || tag === 'textarea' || tag === 'select' || (el as HTMLElement)?.isContentEditable) {
        return
      }
      event.preventDefault()
      inputRef.current?.focus()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  return (
    <AutoComplete
      className="app-header-search"
      options={options}
      style={{ width: 220 }}
      placeholder="搜索模块（/）"
      // 只按显示名过滤：路由是内部的，按它匹配会让人以为搜到的是别的东西。
      filterOption={(input, option) =>
        String(option?.label ?? '')
          .toLowerCase()
          .includes(input.trim().toLowerCase())
      }
      notFoundContent="没有匹配的模块"
      onSelect={(value: string) => navigate(value)}
    >
      <Input ref={inputRef} prefix={<SearchOutlined style={{ opacity: 0.55 }} />} allowClear />
    </AutoComplete>
  )
}
