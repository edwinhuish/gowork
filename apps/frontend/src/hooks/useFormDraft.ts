import { useCallback, useEffect } from 'react'

const DRAFT_PREFIX = 'gowork:draft:'

export interface FormDraft<T> {
  /** 取出草稿；没有草稿或内容已损坏时返回 null。 */
  take: () => T | null
  /** 清掉草稿。保存成功、用户主动取消时调用。 */
  clear: () => void
}

/**
 * useFormDraft 让「填了一半的新建表单」在被打断后还能找回来。
 *
 * 典型场景：token 过期被踢回登录页，重新登录后那份还没提交的单据内容不该凭空消失。
 *
 * 只用于新建，不用于编辑：编辑的对象本身存在于服务端，重新进去能看到原始数据，
 * 而拿草稿盖上去会掩盖别人期间的修改（当前没有版本号，判断不了谁更新）。
 *
 * 存 sessionStorage 而不是 localStorage：
 * - 关掉标签页就该作废，一份没提交的外贸单据不该在下次开机时冒出来
 * - 天然按标签页隔离，双开两个窗口不会互相覆盖
 *
 * active 为 false 时不写入，避免用初始的空表单把已有草稿冲掉。
 */
export function useFormDraft<T>(key: string, form: T, active: boolean): FormDraft<T> {
  const storageKey = DRAFT_PREFIX + key

  useEffect(() => {
    if (!active) return
    try {
      sessionStorage.setItem(storageKey, JSON.stringify(form))
    } catch {
      // 隐私模式或配额已满时写不进去。草稿是锦上添花，不该影响正常填单。
    }
  }, [storageKey, form, active])

  const take = useCallback(() => {
    try {
      const raw = sessionStorage.getItem(storageKey)
      return raw ? (JSON.parse(raw) as T) : null
    } catch {
      return null
    }
  }, [storageKey])

  const clear = useCallback(() => {
    try {
      sessionStorage.removeItem(storageKey)
    } catch {
      // 同上：清不掉也不影响主流程。
    }
  }, [storageKey])

  return { take, clear }
}
