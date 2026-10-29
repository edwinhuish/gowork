import { useState } from 'react'
import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Button, Descriptions } from 'antd'

import { AttachmentPanel } from '@/components/attachment-panel'
import { Empty, EntityTypeTag, Money, RoleTag, StatusTag } from '@/components/fields'
import { Partner360Panel } from '@/components/partner-360-panel'
import { PartnerMergeModal } from '@/components/partner-merge-modal'
import type { Partner } from '@/types'
import { formatTime } from '@/utils/format'

export function CustomerShow() {
  const { result, query } = useShow<Partner>({ resource: 'customers' })
  const [mergeOpen, setMergeOpen] = useState(false)

  return (
    <Show
      isLoading={query.isLoading}
      title={result?.name ?? '客户详情'}
      // 合并入口放在详情页而不是列表：点错一行就并掉一家客户，代价太大。
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
            // 定点字符串："0" 也是真值，所以要先转 number 再判断"不限"。
            children:
              result && Number(result.credit_limit) > 0 ? (
                <Money value={result.credit_limit} />
              ) : (
                '不限'
              ),
          },
          { key: 'currency_code', label: '默认币种', children: result?.currency_code || <Empty /> },
          {
            key: 'owner',
            label: '负责人',
            // 未分配是正常状态（公共池），写"未分配"而不是空白：
            // 空白会让人以为是接口没返回。
            children: result?.owner_name || <span className="cell-sub">未分配</span>,
          },
          { key: 'address', label: '地址', span: 2, children: result?.address || <Empty /> },
          { key: 'remark', label: '备注', span: 2, children: result?.remark || <Empty /> },
          { key: 'created_at', label: '创建时间', children: formatTime(result?.created_at) },
          { key: 'updated_at', label: '更新时间', children: formatTime(result?.updated_at) },
        ]}
      />

      {/* 主体上的附件是长期有效的那类：营业执照、开户信息、验厂报告。
          与出货单证不同，它们不跟着某一票货走。 */}
      {result?.id ? (
        <AttachmentPanel entityType="partner" entityId={result.id} title="资质与证件" />
      ) : null}

      {/* 客户 360：订单、出货、应收、跟进、商机一次取全，免得为了看"这家什么情况"
          去五个列表各筛一遍。只挂客户侧——供应商那侧这五块基本是空的。 */}
      {result?.id ? <Partner360Panel partnerId={result.id} /> : null}

      {result && (
        <PartnerMergeModal
          open={mergeOpen}
          partner={result}
          resource="customers"
          onClose={() => setMergeOpen(false)}
        />
      )}
    </Show>
  )
}
