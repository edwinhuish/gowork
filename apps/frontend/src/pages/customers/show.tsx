import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Descriptions } from 'antd'

import { Empty, EntityTypeTag, Money, RoleTag, StatusTag } from '@/components/fields'
import type { Partner } from '@/types'
import { formatTime } from '@/utils/format'

export function CustomerShow() {
  const { result, query } = useShow<Partner>({ resource: 'customers' })

  return (
    <Show isLoading={query.isLoading} title={result?.name ?? '客户详情'}>
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
          { key: 'country', label: '国家/地区', children: result?.country || <Empty /> },
          { key: 'status', label: '状态', children: <StatusTag value={result?.status} /> },
          { key: 'contact', label: '联系人', children: result?.contact || <Empty /> },
          { key: 'phone', label: '电话', children: result?.phone || <Empty /> },
          { key: 'email', label: '邮箱', children: result?.email || <Empty /> },
          { key: 'trade_term', label: '贸易术语', children: result?.trade_term || <Empty /> },
          { key: 'settlement_method', label: '结算方式', children: result?.settlement_method || <Empty /> },
          {
            key: 'credit_days',
            label: '账期',
            children: result ? `${result.credit_days} 天` : <Empty />,
          },
          {
            key: 'credit_limit',
            label: '信用额度',
            children: result?.credit_limit ? <Money value={result.credit_limit} /> : '不限',
          },
          { key: 'currency_code', label: '默认币种', children: result?.currency_code || <Empty /> },
          { key: 'address', label: '地址', span: 2, children: result?.address || <Empty /> },
          { key: 'remark', label: '备注', span: 2, children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />
    </Show>
  )
}
