import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions } from 'antd'

import { Empty, EntityTypeTag, RoleTag, StatusTag } from '@/components/fields'
import type { Party } from '@/types'
import { formatTime } from '@/utils/format'

export function SupplierShow() {
  const { result, query } = useShow<Party>({ resource: 'suppliers' })

  return (
    <Show isLoading={query.isLoading} title={result?.name ?? '供应商详情'}>
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
          { key: 'address', label: '地址', span: 2, children: result?.address || <Empty /> },
          { key: 'remark', label: '备注', span: 2, children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />
    </Show>
  )
}
