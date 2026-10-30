import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { PictureOutlined, UploadOutlined } from '@ant-design/icons'
import { useInvalidate } from '@refinedev/core'
import {
  Button,
  Card,
  Empty,
  Modal,
  Popconfirm,
  Space,
  Tag,
  Typography,
  message,
} from 'antd'

import { productImageApi } from '@/api/product-images'
import { AuthedImage } from '@/components/authed-image'
import type { ProductImage } from '@/types'
import { formatBytes } from '@/utils/format'

/** 与后端 service.allowedProductImageTypes 一致；前端这份只为了少撞一次 400。 */
const ACCEPT = '.jpg,.jpeg,.png,.webp'
const ACCEPT_EXT = /\.(jpe?g|png|webp)$/i
const MAX_SIZE = 5 * 1024 * 1024
const MAX_IMAGES = 12
/** 后端单批上限；选了 12 张时前端自动分三批。 */
const MAX_BATCH = 4

function validate(file: File): string | null {
  if (!ACCEPT_EXT.test(file.name)) {
    return `${file.name}：只支持 JPG / PNG / WebP`
  }
  if (file.size > MAX_SIZE) {
    return `${file.name}：超过单张 5MB`
  }
  return null
}

/**
 * 产品图片面板：多图上传、排序换主图、替换与删除。
 *
 * 主图没有独立开关：**第一张就是主图**，「设为主图」就是把那张移到第一位。
 * 两个来源（标记 + 顺序）迟早会出现「标记在这张、顺序却在第三位」的记录，
 * 而界面上分不清哪张才是列表里显示的那张。
 *
 * 排序走全量提交：后端收到的 id 集合与库里不一致时返回 409，
 * 前端提示刷新而不是猜着合并。
 */
export function ProductImagesPanel({ productId }: { productId: number }) {
  const [list, setList] = useState<ProductImage[]>([])
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [preview, setPreview] = useState<ProductImage | null>(null)
  const [replaceTarget, setReplaceTarget] = useState<ProductImage | null>(null)
  const uploadRef = useRef<HTMLInputElement>(null)
  const replaceRef = useRef<HTMLInputElement>(null)
  const invalidate = useInvalidate()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setList(await productImageApi.list(productId))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '图片列表加载失败')
    } finally {
      setLoading(false)
    }
  }, [productId])

  useEffect(() => {
    void load()
  }, [load])

  /**
   * 产品列表与详情的缓存都要失效：列表页的缩略图来自 primary_image，
   * 排序或删除后不失效，列表会继续显示旧主图——那比不显示更让人怀疑。
   */
  const refreshProduct = useCallback(async () => {
    await invalidate({ resource: 'products', invalidates: ['list', 'detail'] })
  }, [invalidate])

  const pickFiles = async (event: ChangeEvent<HTMLInputElement>) => {
    const picked = Array.from(event.target.files ?? [])
    // 清空 value：否则再选同一个文件不会触发 change。
    event.target.value = ''
    if (!picked.length) return

    const remaining = MAX_IMAGES - list.length
    if (remaining <= 0) {
      message.error(`每个产品最多 ${MAX_IMAGES} 张图片`)
      return
    }

    const accepted: File[] = []
    picked.forEach((file) => {
      const problem = validate(file)
      if (problem) {
        message.error(problem)
        return
      }
      accepted.push(file)
    })

    const batch = accepted.slice(0, remaining)
    if (accepted.length > batch.length) {
      message.warning(`最多还能传 ${remaining} 张，多余的未提交`)
    }
    if (!batch.length) return

    setUploading(true)
    try {
      // 分批提交：后端单批最多 4 张（4×5MB 低于请求体上限）。
      // 批与批之间不是原子的，失败时已成功的批次会留在服务端，所以失败后要重拉列表。
      for (let i = 0; i < batch.length; i += MAX_BATCH) {
        await productImageApi.upload(productId, batch.slice(i, i + MAX_BATCH))
      }
      message.success(`已上传 ${batch.length} 张`)
      await load()
      await refreshProduct()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '上传失败')
      await load()
      await refreshProduct()
    } finally {
      setUploading(false)
    }
  }

  const applyOrder = async (ids: number[]) => {
    setBusy(true)
    try {
      setList(await productImageApi.reorder(productId, ids))
      await refreshProduct()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '调整顺序失败')
      await load()
    } finally {
      setBusy(false)
    }
  }

  const move = (index: number, delta: number) => {
    const next = [...list]
    const [item] = next.splice(index, 1)
    next.splice(index + delta, 0, item)
    void applyOrder(next.map((image) => image.id))
  }

  /** 设为主图 = 移到第一位；主图由顺序推导，服务端没有另一个标记要改。 */
  const setPrimary = (index: number) => {
    if (index === 0) return
    const next = [...list]
    const [item] = next.splice(index, 1)
    next.unshift(item)
    void applyOrder(next.map((image) => image.id))
  }

  const remove = async (image: ProductImage) => {
    setBusy(true)
    try {
      await productImageApi.remove(productId, image.id)
      message.success('已删除图片')
      await load()
      await refreshProduct()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    } finally {
      setBusy(false)
    }
  }

  const beginReplace = (image: ProductImage) => {
    setReplaceTarget(image)
    replaceRef.current?.click()
  }

  const pickReplace = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || !replaceTarget) return

    const problem = validate(file)
    if (problem) {
      message.error(problem)
      setReplaceTarget(null)
      return
    }

    setBusy(true)
    try {
      await productImageApi.replace(productId, replaceTarget.id, file)
      message.success('已替换，位置与主图身份不变')
      await load()
      await refreshProduct()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '替换失败')
    } finally {
      setBusy(false)
      setReplaceTarget(null)
    }
  }

  const full = list.length >= MAX_IMAGES

  return (
    <Card
      title="产品图片"
      style={{ marginTop: 16 }}
      loading={loading && list.length === 0}
      extra={
        <Button
          type="primary"
          icon={<UploadOutlined />}
          loading={uploading}
          disabled={full || busy}
          onClick={() => uploadRef.current?.click()}
        >
          上传图片
        </Button>
      }
    >
      <input
        ref={uploadRef}
        type="file"
        accept={ACCEPT}
        multiple
        hidden
        onChange={(event) => void pickFiles(event)}
      />
      <input
        ref={replaceRef}
        type="file"
        accept={ACCEPT}
        hidden
        onChange={(event) => void pickReplace(event)}
      />

      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        第一张为主图，产品列表与详情都用它；用「设为主图」或上移把它排到最前。
        单张不超过 5MB，每个产品最多 {MAX_IMAGES} 张；只收 JPG / PNG / WebP，
        类型与大小由服务端复核，改后缀骗不过去。
      </Typography.Paragraph>

      {list.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="还没有图片。上传后第一张自动作为主图。"
        >
          <Button
            type="primary"
            icon={<UploadOutlined />}
            loading={uploading}
            onClick={() => uploadRef.current?.click()}
          >
            上传第一张
          </Button>
        </Empty>
      ) : (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12 }}>
          {list.map((image, index) => (
            <div
              key={image.id}
              style={{
                width: 188,
                border: '1px solid #f0f0f0',
                borderRadius: 8,
                overflow: 'hidden',
                background: '#fff',
              }}
            >
              <div style={{ position: 'relative', height: 142, background: '#fafafa' }}>
                <AuthedImage
                  path={image.thumb_url}
                  alt={image.file_name}
                  style={{
                    width: '100%',
                    height: 142,
                    objectFit: 'cover',
                    cursor: 'zoom-in',
                    display: 'block',
                  }}
                  onClick={() => setPreview(image)}
                />
                {index === 0 ? (
                  <Tag
                    color="blue"
                    style={{ position: 'absolute', top: 8, left: 8, marginInlineEnd: 0 }}
                  >
                    主图
                  </Tag>
                ) : null}
              </div>

              <div style={{ padding: '8px 10px 4px' }}>
                <div
                  className="cell-strong"
                  title={image.file_name}
                  style={{
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {image.file_name}
                </div>
                <div className="cell-sub">
                  {image.width}×{image.height} · {formatBytes(image.size)}
                </div>
              </div>

              <Space size={0} wrap style={{ padding: '0 4px 4px' }}>
                <Button
                  size="small"
                  type="link"
                  disabled={index === 0 || busy}
                  onClick={() => move(index, -1)}
                >
                  上移
                </Button>
                <Button
                  size="small"
                  type="link"
                  disabled={index === 0 || busy}
                  onClick={() => setPrimary(index)}
                >
                  设为主图
                </Button>
                <Button
                  size="small"
                  type="link"
                  disabled={index === list.length - 1 || busy}
                  onClick={() => move(index, 1)}
                >
                  下移
                </Button>
                <Button size="small" type="link" disabled={busy} onClick={() => beginReplace(image)}>
                  替换
                </Button>
                <Popconfirm
                  title="删除这张图片？"
                  description={
                    index === 0
                      ? '它是主图，删除后由下一张接任；列表与详情的缩略图会立即跟着换。'
                      : '删除后不可恢复，需要时重新上传。'
                  }
                  okText="删除"
                  cancelText="取消"
                  onConfirm={() => void remove(image)}
                >
                  <Button size="small" type="link" danger disabled={busy}>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            </div>
          ))}
        </div>
      )}

      {full ? (
        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          已达到 {MAX_IMAGES} 张上限。需要换图时先删一张，或用「替换」保留位置。
        </Typography.Paragraph>
      ) : null}

      <Modal
        open={preview !== null}
        title={preview?.file_name}
        footer={null}
        width={720}
        onCancel={() => setPreview(null)}
      >
        {preview ? (
          <AuthedImage
            path={preview.url}
            alt={preview.file_name}
            style={{ width: '100%', display: 'block', marginTop: 8 }}
          />
        ) : null}
        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          <PictureOutlined /> {preview?.width}×{preview?.height} ·{' '}
          {formatBytes(preview?.size ?? 0)} · 这是原图，列表里用的是缩略图
        </Typography.Paragraph>
      </Modal>
    </Card>
  )
}
