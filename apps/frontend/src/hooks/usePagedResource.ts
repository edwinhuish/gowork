import { useCallback, useEffect, useRef, useState } from 'react'

import type { PageResult } from '@/types'

const PAGE_SIZE = 15

interface FetchParams {
  page: number
  page_size: number
  keyword: string
}

interface PagedResource<T> {
  items: T[]
  total: number
  page: number
  keyword: string
  loading: boolean
  error: string
  setPage: (page: number) => void
  setKeyword: (keyword: string) => void
  reload: () => void
}

/**
 * 列表页的通用状态机：分页、关键字、加载态、错误态与竞态处理。
 *
 * 各页面只提供自己的 fetchPage 闭包，额外筛选条件在闭包里拼接。
 * fetchPage 存进 ref，保证 effect 里总是用到最新闭包，不受 stale closure 影响。
 */
export function usePagedResource<T>(
  fetchPage: (p: FetchParams) => Promise<PageResult<T>>,
): PagedResource<T> {
  const [items, setItems] = useState<T[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [keyword, setKeywordState] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [tick, setTick] = useState(0)

  const fetchRef = useRef(fetchPage)
  fetchRef.current = fetchPage

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')

    fetchRef
      .current({ page, page_size: PAGE_SIZE, keyword })
      .then((res) => {
        if (cancelled) return
        setItems(res.list)
        setTotal(res.total)
      })
      .catch((err: Error) => {
        if (cancelled) return
        setItems([])
        setTotal(0)
        setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [page, keyword, tick])

  const setKeyword = useCallback((value: string) => {
    setPage(1)
    setKeywordState(value)
  }, [])

  const reload = useCallback(() => setTick((n) => n + 1), [])

  return { items, total, page, keyword, loading, error, setPage, setKeyword, reload }
}

export { PAGE_SIZE }
