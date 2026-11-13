import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useTable } from '@refinedev/antd'
import type { useTableProps, useTableReturnType } from '@refinedev/antd'
import type { BaseRecord, HttpError } from '@refinedev/core'
import type { TableProps } from 'antd'

/**
 * 无限滚动的数据源适配层。
 *
 * 列表页原本靠 `Table` 的分页条一次拿一页。分页条退场之后，「还有没有下一批、
 * 下一批怎么来」必须有人回答，而前端的数据来源有三种，不能只写一份：
 *
 *   1. `useInfiniteList`：走 refine 的 `useTable`（20 个 CRUD 列表页）。
 *      一页一页地请求，按页号攒成一份连续列表。
 *   2. `useClientFeed`：数据本来就一次取满（`pagination: { mode: 'off' }`
 *      或 `useList` 全量），只是行数到了两三百。这里只负责分批揭示，
 *      不发请求。
 *   3. `usePagedFeed`：自己手写分页参数的页面（操作日志、条款模板），
 *      用 TanStack 的 `useInfiniteQuery` 管页序与重试。
 *
 * 三者产出同一个 `InfiniteFeed`，所以底部的 `InfiniteFooter`、列表页外壳
 * `PageList` 与卡片视图都不必知道数据是怎么来的。
 */

/** 一屏大约能放下的行数：表格 15 行，卡片 12 张（能排满一行）。 */
export const LIST_PAGE_SIZE = 15
export const CARD_PAGE_SIZE = 12

export interface InfiniteFeed<T> {
  /** 已经攒下来的全部行，直接给 Table 或卡片网格。 */
  rows: T[]
  /** 后端（或本地数组）给出的总数，用于「已加载全部 N 条」与判断还有没有下一批。 */
  total: number
  hasMore: boolean
  /** 正在取下一批：此时列表照旧显示，只在底部多一个转圈。 */
  loadingMore: boolean
  error: Error | null
  loadMore: () => void
  retry: () => void
}

interface PageStore<T> {
  /** 这一批页属于哪一组筛选 + 排序；条件一变，整份作废。 */
  signature: string
  pages: Map<number, T[]>
  total: number
}

/**
 * 按页号升序拼成一份列表，并按 id 去重。
 *
 * 去重不是多余：两次请求之间有人新增了记录，第二页的第一条会与第一页的最后一条
 * 重叠。没有这一层，同一条会在列表里出现两遍，而 React 的 key 重复还会让
 * 表格渲染出错。
 */
function mergePages<T extends BaseRecord>(pages: Map<number, T[]>): T[] {
  const seen = new Set<string>()
  const merged: T[] = []

  for (const index of [...pages.keys()].sort((a, b) => a - b)) {
    for (const row of pages.get(index) ?? []) {
      const key = row.id === undefined || row.id === null ? '' : String(row.id)
      if (key) {
        if (seen.has(key)) continue
        seen.add(key)
      }
      merged.push(row)
    }
  }

  return merged
}

type AntdSorterEntry = {
  field?: unknown
  columnKey?: unknown
  order?: unknown
} & Record<string, unknown>

/**
 * 把 antd sorter 的 `columnKey` 换成真正的字段名。
 *
 * antd 给 `<Table.Column>`（没有显式 `key` 的写法）生成的 `key` 是列在树里的
 * **路径**（实测是 `.2` `.7` 这种），而 refine 映射 CrudSort 时优先取
 * `columnKey ?? field` —— 于是 `sort_field` 变成了 `.2`，后端白名单认不出来，
 * 静默忽略，表现是「点了表头，箭头动了，顺序没变」。
 *
 * 这里以 `field`（也就是列上写的 dataIndex）为准盖掉它。拿不到 dataIndex
 * （只有 render 的列）时原样返回，不去猜。
 */
function withSortField<T>(sorter: T): T {
  const list = Array.isArray(sorter) ? sorter : [sorter]
  const fixed = list.map((item) => {
    if (!item || typeof item !== 'object') return item
    const entry = item as AntdSorterEntry
    const field = Array.isArray(entry.field) ? entry.field.join('.') : entry.field
    if (field === undefined || field === null || field === '') return item
    return { ...entry, columnKey: String(field) }
  })
  return (Array.isArray(sorter) ? fixed : fixed[0]) as T
}

/**
 * refine `useTable` 的无限滚动版本：参数与返回值完全一致，只多一个 `feed`。
 *
 * `tableProps.dataSource` 换成了累计出来的行，`pagination` 固定为 `false`
 * （分页条不再渲染），排序与筛选仍然走 refine：表头点击、搜索表单、URL 同步
 * 都不受影响，唯一的变化是「换条件之后从第一页重新攒」，而不是跳到第 N 页。
 */
export function useInfiniteList<T extends BaseRecord = BaseRecord, TSearchVariables = unknown>(
  props: useTableProps<T, HttpError, TSearchVariables, T>,
): useTableReturnType<T, HttpError, TSearchVariables> & { feed: InfiniteFeed<T> } {
  const table = useTable<T, HttpError, TSearchVariables>(props)
  const { tableQuery, currentPage, setCurrentPage, pageSize, filters, sorters } = table

  // 筛选与排序的全部真相都在 refine 里，序列化后当作「这批页属于哪一组条件」的指纹。
  const signature = useMemo(() => JSON.stringify({ filters, sorters }), [filters, sorters])

  const storeRef = useRef<PageStore<T>>({ signature, pages: new Map(), total: 0 })
  // 同步的"正在取"闸门：state 要等一次渲染才生效，滚动事件可能在一帧里触发两次。
  const busyRef = useRef(false)
  const [rows, setRows] = useState<T[]>([])
  const [total, setTotal] = useState(0)
  const [exhausted, setExhausted] = useState(false)

  // 刷新页面时 URL 里可能还留着 currentPage=3，但我们只有第 3 页那一批数据。
  // 无限滚动没有"第几页"这个概念，进来一律从第一页重新累计。
  useEffect(() => {
    if (currentPage !== 1) setCurrentPage(1)
    // 只在挂载时跑一次：之后的页码跳转都由 loadMore 与下面的重置逻辑驱动。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 筛选或排序变了：已累计的行全部作废，回第一页重新装。
  useEffect(() => {
    if (storeRef.current.signature === signature) return
    storeRef.current = { signature, pages: new Map(), total: 0 }
    busyRef.current = false
    setRows([])
    setTotal(0)
    setExhausted(false)
    if (currentPage !== 1) setCurrentPage(1)
  }, [signature, currentPage, setCurrentPage])

  // 一页数据到手：写进 store，再按页序拼成一份连续列表。
  useEffect(() => {
    const response = tableQuery.data
    // refine 给整个 QueryClient 配了 placeholderData: keepPreviousData —— 翻页请求
    // 发出后 `data` 先返回**上一页**的内容，`isPlaceholderData` 为 true。
    // 不挡掉它，第 2 页就会被写成第 1 页的副本，整张表原地不动。
    if (!response || tableQuery.isPlaceholderData) return
    const store = storeRef.current
    if (store.signature !== signature) return
    const pageRows = (response.data ?? []) as T[]
    store.pages.set(currentPage, pageRows)
    store.total = response.total ?? pageRows.length
    // 删过行之后总页数会变少，越界的那几页要丢掉，否则列表尾部挂着已删除的记录。
    const lastPage = Math.max(1, Math.ceil(store.total / Math.max(pageSize, 1)))
    for (const key of [...store.pages.keys()]) {
      if (key > lastPage) store.pages.delete(key)
    }
    const merged = mergePages(store.pages)
    setRows(merged)
    setTotal(store.total)
    // 短页即末页：后端按 page_size 切，最后一页一定不满。
    setExhausted(pageRows.length < pageSize || merged.length >= store.total)
  }, [tableQuery.data, tableQuery.isPlaceholderData, currentPage, pageSize, signature])

  const hasMore = !exhausted

  const loadMore = useCallback(() => {
    // 出错之后不再自动追加：滚到底会变成"失败一次、再试一次"的死循环，
    // 让用户点重试，重试的是同一页。
    if (busyRef.current || !hasMore || tableQuery.isError) return
    busyRef.current = true
    setCurrentPage((page) => page + 1)
  }, [hasMore, setCurrentPage, tableQuery.isError])

  useEffect(() => {
    // 请求结束（成功或失败）才放闸；fetching 期间必须一直锁着。
    if (!tableQuery.isFetching) busyRef.current = false
  }, [tableQuery.isFetching, tableQuery.data, tableQuery.isError])

  const retry = useCallback(() => {
    void tableQuery.refetch()
  }, [tableQuery.refetch])

  const feed = useMemo<InfiniteFeed<T>>(
    () => ({
      rows,
      total,
      hasMore,
      // 首批也算"正在加载"：否则首屏那一两秒里，底部会先闪一个「加载更多」按钮。
      loadingMore: tableQuery.isFetching,
      error: (tableQuery.error as Error | null) ?? null,
      loadMore,
      retry,
    }),
    [rows, total, hasMore, tableQuery.isFetching, tableQuery.error, loadMore, retry],
  )

  const tableProps: TableProps<T> = {
    ...table.tableProps,
    dataSource: rows,
    // 分页条彻底退场：底部那个 InfiniteFooter 是唯一的"还有没有下一批"入口。
    pagination: false,
    loading: rows.length === 0 && tableQuery.isFetching,
    onChange: (_pagination, tableFilters, sorter, extra) => {
      // 点表头排序 / 列筛选之后必然回到第一页：停在第 3 页看新条件只会看到空表。
      // 这里显式把 current 压到 1，也顺手把 pageSize 钉住（refine 在没有 pageSize 时
      // 会退回它自己的默认值 10）。
      table.tableProps.onChange?.(
        { current: 1, pageSize },
        tableFilters,
        withSortField(sorter),
        extra,
      )
    },
  }

  return { ...table, tableProps, feed }
}

/**
 * 数据已经全在手里（`mode: 'off'` 或 `useList` 全量）时的分批揭示。
 *
 * 这类页面原本一次铺出两三百行，非分页不可的地方其实是首屏渲染。这里不发请求，
 * 只把前 `step` 行交给表格，滚到底再放下一批 —— 与真正的无限滚动看起来一样，
 * 代价是总数一开始就知道，结束提示也就准确。
 */
export function useClientFeed<T extends object>(
  all: T[],
  step = LIST_PAGE_SIZE,
  enabled = true,
): InfiniteFeed<T> {
  // 条件一变（行数或首尾两条换了）就回到第一批。用首尾 id 而不是数组引用：
  // 引用每次请求都可能变，删掉中间一行不该把用户弹回列表顶部。
  const signature = useMemo(() => {
    if (all.length === 0) return '0'
    const first = (all[0] as { id?: unknown }).id
    const last = (all[all.length - 1] as { id?: unknown }).id
    return `${all.length}:${String(first ?? '')}:${String(last ?? '')}`
  }, [all])

  const [visible, setVisible] = useState(step)

  useEffect(() => {
    setVisible(step)
  }, [signature, step])

  const clamped = Math.min(visible, all.length)
  const loadMore = useCallback(() => {
    setVisible((count) => count + step)
  }, [step])

  return {
    rows: enabled ? all.slice(0, clamped) : all,
    total: all.length,
    hasMore: enabled && clamped < all.length,
    loadingMore: false,
    error: null,
    loadMore,
    // 本地数据没有"失败"这一态，重试就是再放一批。
    retry: loadMore,
  }
}

export interface PagedFeed<T> extends InfiniteFeed<T> {
  /** 首批还在路上：列表要显示骨架，而不是"没有数据"。 */
  isLoading: boolean
}

/**
 * 手写分页参数的接口（操作日志、条款模板）的无限滚动版本。
 *
 * 这类页面本来就自己拼 `page` / `page_size`，交给 `useInfiniteQuery` 之后，
 * 页序、并发去重与"还有没有下一页"都由它管，页面只写一个 `fetchPage`。
 */
export function usePagedFeed<T extends BaseRecord>({
  queryKey,
  pageSize = LIST_PAGE_SIZE,
  fetchPage,
}: {
  queryKey: readonly unknown[]
  pageSize?: number
  fetchPage: (page: number) => Promise<{ list: T[]; total: number }>
}): PagedFeed<T> {
  // 页面每次渲染都会新建一个 fetchPage 闭包，放进 queryFn 的依赖会变成无限重取。
  const fetchRef = useRef(fetchPage)
  fetchRef.current = fetchPage

  const query = useInfiniteQuery({
    queryKey: [...queryKey, pageSize],
    initialPageParam: 1,
    queryFn: ({ pageParam }) => fetchRef.current(pageParam),
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((count, page) => count + page.list.length, 0)
      const total = lastPage.total ?? loaded
      if (lastPage.list.length === 0 || loaded >= total) return undefined
      return allPages.length + 1
    },
  })

  const pages = query.data?.pages
  const rows = useMemo(() => pages?.flatMap((page) => page.list) ?? [], [pages])
  const total = pages?.[0]?.total ?? 0

  const loadMore = useCallback(() => {
    if (!query.hasNextPage || query.isFetchingNextPage || query.isError) return
    void query.fetchNextPage()
  }, [query.hasNextPage, query.isFetchingNextPage, query.isError, query.fetchNextPage])

  const retry = useCallback(() => {
    // 失败的是某一页；refetch 会按顺序把已有的页重跑一遍，失败的那一页就在其中。
    void query.refetch()
  }, [query.refetch])

  return {
    rows,
    total,
    hasMore: query.hasNextPage,
    loadingMore: query.isFetchingNextPage || (query.isFetching && rows.length > 0),
    error: (query.error as Error | null) ?? null,
    loadMore,
    retry,
    isLoading: query.isLoading,
  }
}
