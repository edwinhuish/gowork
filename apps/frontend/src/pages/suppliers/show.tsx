import { useState } from 'react'
import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Button, Descriptions, Space } from 'antd'

import { AttachmentPanel } from '@/components/attachment-panel'
import { PartnerAddressPanel } from '@/components/partner-address-panel'
import { Empty, EntityTypeTag, RoleTag } from '@/components/fields'
import { PartnerMergeModal } from '@/components/partner-merge-modal'
import { PartnerOwnerModal } from '@/components/partner-owner-modal'
import type { Partner } from '@/types'
import { formatTime } from '@/utils/format'

export function SupplierShow() {
  const { result, query } = useShow<Partner>({ resource: 'suppliers' })
  const [mergeOpen, setMergeOpen] = useState(false)
  const [ownerOpen, setOwnerOpen] = useState(false)

  return (
    <Show
      isLoading={query.isLoading}
      title={result?.name ?? '供应商详情'}
      headerButtons={
        result ? (
          <Space>
            <Button onClick={() => setOwnerOpen(true)}>转移负责人</Button>
            <Button onClick={() => setMergeOpen(true)}>合并到其他主体</Button>
          </Space>
        ) : null
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
          { key: 'contact', label: '联系人', children: result?.contact || <Empty /> },
          { key: 'phone', label: '电话', children: result?.phone || <Empty /> },
          { key: 'email', label: '邮箱', children: result?.email || <Empty /> },
          {
            key: 'owner',
            label: '负责人',
            children: result?.owner_name || <span className="cell-sub">未分配</span>,
          },
          // 'filled'：占满这一行剩下的列。column 是响应式的（窄屏只有 1 列），
          // 写死 2 会超出列数 —— antd 告警，这一格还会被裁回 1 列。
          { key: 'address', label: '地址', span: 'filled', children: result?.address || <Empty /> },
          { key: 'remark', label: '备注', span: 'filled', children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      {/* 供应商也要多地址：采购合同的收货地址常常是工厂而不是注册地。 */}
      {result?.id ? <PartnerAddressPanel partnerId={result.id} /> : null}

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

      {result && (
        <PartnerOwnerModal
          open={ownerOpen}
          partner={result}
          onClose={() => setOwnerOpen(false)}
          onChanged={() => {
            void query.refetch()
          }}
        />
      )}
    </Show>
  )
}
