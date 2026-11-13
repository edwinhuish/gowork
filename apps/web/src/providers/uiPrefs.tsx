import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import type { ColorMode, SiderMode } from '@/theme'

/**
 * 界面偏好：亮/暗色 + 标准/紧凑密度 + 侧栏配色。
 *
 * 这三项只影响观感，不影响业务数据，所以存在 localStorage 而不是后端：
 * 换台机器重新选一次比为一个开关加一张表划算。
 *
 * 侧栏配色单独一项而不是复用 mode：侧栏和内容区是两套主题，允许一深一浅
 * （深色侧栏配亮色内容是最常见的一种）。默认 follow，即"用户不表态就跟页面走"。
 *
 * 偏好必须落在 ConfigProvider 的**外层**：ConfigProvider 的 theme 由它算出来，
 * 反过来包就会读到上一帧的值，切主题时整页闪一次旧颜色。
 */

export interface UiPrefs {
  mode: ColorMode
  compact: boolean
  siderMode: SiderMode
}

interface UiPrefsValue extends UiPrefs {
  setMode: (mode: ColorMode) => void
  toggleMode: () => void
  setCompact: (compact: boolean) => void
  toggleCompact: () => void
  setSiderMode: (mode: SiderMode) => void
}

const STORAGE_KEY = 'gowork.ui-prefs'

const DEFAULT_PREFS: UiPrefs = { mode: 'light', compact: false, siderMode: 'follow' }

const SIDER_MODES: SiderMode[] = ['follow', 'light', 'dark']

function readSiderMode(value: unknown): SiderMode {
  return SIDER_MODES.includes(value as SiderMode) ? (value as SiderMode) : DEFAULT_PREFS.siderMode
}

/**
 * 首次进入跟随系统：系统是深色就先用深色，用户手动切过之后以存储为准。
 * 解析失败一律回落默认值 —— 一个坏掉的偏好串不该让整页白屏。
 */
function readInitialPrefs(): UiPrefs {
  if (typeof window === 'undefined') return DEFAULT_PREFS
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<UiPrefs> | null
      if (parsed?.mode === 'dark' || parsed?.mode === 'light') {
        return {
          mode: parsed.mode,
          compact: Boolean(parsed.compact),
          // 老版本存过的偏好里没有这一项：缺省就是 follow。
          siderMode: readSiderMode(parsed.siderMode),
        }
      }
    }
  } catch {
    // 存储被禁用或内容损坏：当作没存过。
  }
  const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
  return { mode: prefersDark ? 'dark' : 'light', compact: false, siderMode: 'follow' }
}

const UiPrefsContext = createContext<UiPrefsValue | null>(null)

export function UiPrefsProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<UiPrefs>(readInitialPrefs)

  /**
   * 同时写三处：
   * - data-theme / data-density 给 index.css 里 antd 覆盖不到的样式做分支；
   * - color-scheme 让浏览器原生控件（滚动条、日期选择、自动填充背景）跟着变，
   *   少了它，暗色模式下滚动条还是一道白条。
   */
  useEffect(() => {
    const root = document.documentElement
    root.dataset.theme = prefs.mode
    root.dataset.density = prefs.compact ? 'compact' : 'standard'
    root.style.colorScheme = prefs.mode
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
    } catch {
      // 隐私模式下写入会抛异常：偏好只在本次会话内有效，不影响使用。
    }
  }, [prefs])

  const setMode = useCallback((mode: ColorMode) => {
    setPrefs((prev) => ({ ...prev, mode }))
  }, [])
  const toggleMode = useCallback(() => {
    setPrefs((prev) => ({ ...prev, mode: prev.mode === 'dark' ? 'light' : 'dark' }))
  }, [])
  const setCompact = useCallback((compact: boolean) => {
    setPrefs((prev) => ({ ...prev, compact }))
  }, [])
  const toggleCompact = useCallback(() => {
    setPrefs((prev) => ({ ...prev, compact: !prev.compact }))
  }, [])
  const setSiderMode = useCallback((siderMode: SiderMode) => {
    setPrefs((prev) => ({ ...prev, siderMode }))
  }, [])

  const value = useMemo<UiPrefsValue>(
    () => ({ ...prefs, setMode, toggleMode, setCompact, toggleCompact, setSiderMode }),
    [prefs, setMode, toggleMode, setCompact, toggleCompact, setSiderMode],
  )

  return <UiPrefsContext.Provider value={value}>{children}</UiPrefsContext.Provider>
}

/** 读取界面偏好。未包 Provider 时抛错，避免默默使用默认值导致开关无效。 */
export function useUiPrefs(): UiPrefsValue {
  const ctx = useContext(UiPrefsContext)
  if (!ctx) throw new Error('useUiPrefs 必须在 UiPrefsProvider 内使用')
  return ctx
}
