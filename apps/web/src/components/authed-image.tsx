import { useEffect, useState, type ImgHTMLAttributes } from 'react'
import { PictureOutlined } from '@ant-design/icons'

import { getToken } from '@/api/client'

/**
 * 带令牌取图。
 *
 * 产品图片接口走 Authorization 头，`<img src="/api/v1/...">` 带不上令牌，
 * 直接写进 src 只会拿到 401。所以先 fetch 成 blob，再用 object URL 显示。
 *
 * 按路径缓存 Promise：列表里同一张缩略图被渲染两次（卡片 + 预览）只请求一次；
 * 路径带 v=updated_at，替换内容后路径变化，缓存不会继续用旧图。
 * 不主动 revoke：object URL 的生命周期跟页面一致，提前 revoke 会让正在显示
 * 的 <img> 变成空白；代价是每张看过的图占一份内存，产品图数量有限。
 */
const cache = new Map<string, Promise<string>>()

export function loadAuthedImage(path: string): Promise<string> {
  const hit = cache.get(path)
  if (hit) return hit

  const task = (async () => {
    const headers = new Headers()
    const token = getToken()
    if (token) headers.set('Authorization', `Bearer ${token}`)
    const res = await fetch(`/api/v1${path}`, { headers })
    if (!res.ok) throw new Error(`图片加载失败 (${res.status})`)
    return URL.createObjectURL(await res.blob())
  })()

  // 失败不缓存：下次进入页面还能重试。
  task.catch(() => {
    cache.delete(path)
  })
  cache.set(path, task)
  return task
}

/** 解析一个取图路径为 object URL。path 为空时什么都不做。 */
export function useAuthedUrl(path?: string): { url?: string; failed: boolean } {
  const [url, setUrl] = useState<string>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!path) {
      setUrl(undefined)
      setFailed(false)
      return
    }
    let alive = true
    setUrl(undefined)
    setFailed(false)
    loadAuthedImage(path)
      .then((value) => {
        if (alive) setUrl(value)
      })
      .catch(() => {
        if (alive) setFailed(true)
      })
    return () => {
      alive = false
    }
  }, [path])

  return { url, failed }
}

interface AuthedImageProps extends Omit<ImgHTMLAttributes<HTMLImageElement>, 'src'> {
  path?: string
  alt: string
}

/**
 * 带令牌的 <img>。加载中显示浅色占位，失败显示图片图标而不是浏览器的破图。
 *
 * 占位与图片用同一份 style：尺寸在加载前后不变，列表不会因为图片到了而跳行。
 */
export function AuthedImage({ path, alt, style, ...rest }: AuthedImageProps) {
  const { url, failed } = useAuthedUrl(path)

  if (failed || !url) {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'var(--ant-color-fill-quaternary)',
          color: 'var(--ant-color-text-quaternary)',
          ...style,
        }}
        aria-label={alt}
        role="img"
      >
        <PictureOutlined />
      </span>
    )
  }

  return <img src={url} alt={alt} style={style} {...rest} />
}
