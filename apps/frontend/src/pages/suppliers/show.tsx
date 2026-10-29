import { useState } from 'react'
import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Button, Descriptions } from 'antd'

import { AttachmentPanel } from '@/components/attachment-panel'
import { Empty, EntityTypeTag, RoleTag, StatusTag } from '@/components/fields'
import { PartnerMergeModal } from '@/components/partner-merge-modal'
import type { Partner } from '@/types'
import { formatTime } from '@/utils/format'

export function SupplierShow() {
  const { result, query } = useShow<Partner>({ resource: 'suppliers' })
  const [mergeOpen, setMergeOpen] = useState(false)

  return (
    <Show
      isLoading={query.isLoading}
      title={result?.name ?? '供应商详情'}
      headerButtons={
        result ? <Button onClick={() => setMergeOpen(true)}>合并到其他主体</Button> : null
      }
    >
      <Descriptions
        column={{ xs: 1, sm: 2, lg: 3 }}
        bordered
        size="middle"
        items={[
          {
            key: 'code',
            label: '编码',
            children: <span className="cell-mono">{result?.code}</span>,
          },
          { key: 'name', label: '名称', children: result?.name || <Empty /> },
          { key: 'role', label: '角色', children: <RoleTag value={result?.role} /> },
          { key: 'entity_type', label: '主体类型', children: <EntityTypeTag value={result?.entity_type} /> },
          { key: 'status', label: '状态', children: <StatusTag value={result?.status} /> },
          { key: 'contact', label: '联系人', children: result?.contact || <Empty /> },
          { key: 'phone', label: '电话', children: result?.phone || <Empty /> },
          { key: 'email', label: '邮箱', children: result?.email || <Empty /> },
          {
            key: 'owner',
            label: '负责人',
            children: result?.owner_name || <span className="cell-sub">未分配</span>,
          },
          { key: 'address', label: '地址', span: 2, children: result?.address || <Empty /> },
          { key: 'remark', label: '备注', span: 2, children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      {/* 供应商侧常见的是 ISO 证书、质检报告、营业执照：同样是长期有效的文件。 */}
      {result?.id ? (
        <AttachmentPanel entityType="partner" entityId={result.id} title="资质与证件" />
      ) : null}

      {result && (
        <PartnerMergeModal
          open={mergeOpen}
          partner={result}
          resource="suppliers"
          onClose={() => setMergeOpen(false)}
        />
      )}
    </Show>
  )
}
