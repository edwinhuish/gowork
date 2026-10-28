import { useCallback, useEffect, useState } from 'react'
import { FilePdfOutlined, FileImageOutlined, UploadOutlined } from '@ant-design/icons'
import { Button, Card, Popconfirm, Space, Table, Tag, Typography, Upload, message } from 'antd'

import { attachmentApi, type AttachmentEntity } from '@/api/attachments'
import { download } from '@/api/client'
import { Empty } from '@/components/fields'
import type { Attachment } from '@/types'
import { formatBytes, formatTime } from '@/utils/format'

/** 与后端 service.allowedAttachmentTypes 一致；前端这份只为了少撞一次 400。 */
const ACCEPT = '.pdf,.jpg,.jpeg,.png,.webp'
const MAX_SIZE = 10 * 1024 * 1024

interface AttachmentPanelProps {
  entityType: AttachmentEntity
  entityId: number
  /** 标题，默认「附件」；页面可以改成「单证附件」这类更贴业务的叫法。 */
  title?: string
}

/**
 * 通用附件面板：挂在任何主体或单据的详情页上。
 *
 * 一个组件而不是每张单据各写一遍：附件的价值在于「翻得到当时那份文件」，
 * 每个页面自己实现一遍上传与下载，最后一定会有一半页面忘了带令牌或忘了限大小。
 *
 * 没有编辑：换文件等于重新传一份再删旧的——就地覆盖会让「这版是客户签的那版吗」
 * 变得无法回答。
 */
export function AttachmentPanel({ entityType, entityId, title = '附件' }: AttachmentPanelProps) {
  const [list, setList] = useState<Attachment[]>([])
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setList(await attachmentApi.list(entityType, entityId))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '附件列表加载失败')
    } finally {
      setLoading(false)
    }
  }, [entityType, entityId])

  useEffect(() => {
    void load()
  }, [load])

  const upload = async (file: File) => {
    // 大小在这里先拦一次：等到传完 10MB 再被服务端拒绝，白等一遍。
    if (file.size > MAX_SIZE) {
      message.error('单个附件不能超过 10 MB')
      return
    }
    setUploading(true)
    try {
      await attachmentApi.upload(entityType, entityId, file)
      message.success(`已上传 ${file.name}`)
      await load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '上传失败')
    } finally {
      setUploading(false)
    }
  }

  const downloadOne = async (record: Attachment) => {
    try {
      // 文件名以服务端响应头为准：中文名只有 filename* 带得回来。
      await download(attachmentApi.downloadPath(record.id), record.file_name)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载失败')
    }
  }

  const remove = async (record: Attachment) => {
    try {
      await attachmentApi.remove(record.id)
      message.success('已删除附件')
      await load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      title={title}
      style={{ marginTop: 16 }}
      extra={
        <Upload
          accept={ACCEPT}
          showUploadList={false}
          disabled={uploading}
          // 交给自己的上传逻辑：antd 的默认上传不带令牌。
          beforeUpload={(file) => {
            void upload(file)
            return false
          }}
        >
          <Button icon={<UploadOutlined />} loading={uploading} type="primary">
            上传附件
          </Button>
        </Upload>
      }
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        提单、报关单、验货报告这类扫描件传这里。只收 PDF / JPG / PNG / WebP，单个文件不超过 10MB
        —— 类型与大小由服务端复核，改个后缀骗不过去。
      </Typography.Paragraph>

      <Table
        dataSource={list}
        loading={loading}
        rowKey="id"
        size="small"
        pagination={false}
        scroll={{ x: 720 }}
        locale={{ emptyText: '还没有附件。货代或海关发来的文件到手后在这里补传。' }}
      >
        <Table.Column
          dataIndex="file_name"
          title="文件"
          render={(value: string, record: Attachment) => (
            <Space size={8}>
              {record.mime_type === 'application/pdf' ? (
                <FilePdfOutlined style={{ color: '#c0392b' }} />
              ) : (
                <FileImageOutlined style={{ color: '#2c7be5' }} />
              )}
              <span className="cell-strong">{value}</span>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="mime_type"
          title="类型"
          width={130}
          render={(value: string) => (
            <Tag>{value.replace('application/', '').replace('image/', '')}</Tag>
          )}
        />
        <Table.Column
          dataIndex="size"
          title="大小"
          width={100}
          align="right"
          render={(value: number) => formatBytes(value)}
        />
        <Table.Column
          dataIndex="remark"
          title="备注"
          width={180}
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          dataIndex="created_at"
          title="上传时间"
          width={170}
          render={(value: string) => formatTime(value)}
        />
        <Table.Column
          title="操作"
          width={130}
          render={(_, record: Attachment) => (
            <Space size={4}>
              <Button size="small" type="link" onClick={() => downloadOne(record)}>
                下载
              </Button>
              <Popconfirm
                title="删除这份附件？"
                description="删除后不在列表里显示；文件本身为追溯保留，需要时可以找管理员恢复。"
                okText="删除"
                cancelText="取消"
                onConfirm={() => remove(record)}
              >
                <Button size="small" type="link" danger>
                  删除
                </Button>
              </Popconfirm>
            </Space>
          )}
        />
      </Table>
    </Card>
  )
}
