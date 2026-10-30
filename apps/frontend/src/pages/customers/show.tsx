import { useState } from 'react'
import { Show } from '@refinedev/antd'
import { useShow } from '@refinedev/core'
import { Button, Descriptions, Space, Tag } from 'antd'

import { AttachmentPanel } from '@/components/attachment-panel'
import { Empty, EntityTypeTag, Money, RoleTag } from '@/components/fields'
import { Partner360Panel } from '@/components/partner-360-panel'
import { PartnerAddressPanel } from '@/components/partner-address-panel'
import { PartnerMergeModal } from '@/components/partner-merge-modal'
import { PartnerOwnerModal } from '@/components/partner-owner-modal'
import type { Partner } from '@/types'
import { formatTime } from '@/utils/format'

export function CustomerShow() {
  const { result, query } = useShow<Partner>({ resource: 'customers' })
  const [mergeOpen, setMergeOpen] = useState(false)
  const [ownerOpen, setOwnerOpen] = useState(false)

  return (
    <Show
      isLoading={query.isLoading}
      title={result?.name ?? '客户详情'}
      // 转移与合并都放在详情页而不是列表：点错一行就能把一家客户划走或并掉，代价太大。
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
          { key: 'country', label: '国家/地区', children: result?.country || <Empty /> },
          { key: 'tax_id', label: '税号', children: result?.tax_id || <Empty /> },
          { key: 'customer_group', label: '客户分组', children: result?.customer_group || <Empty /> },
          { key: 'region', label: '销售区域', children: result?.region || <Empty /> },
          {
            key: 'grade',
            label: '客户分级',
            // 未分级是常态（存量数据全在这一档），写"未分级"而不是空白。
            children: result?.grade ? <Tag bordered={false}>{result.grade}</Tag> : <span className="cell-sub">未分级</span>,
          },
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

      {/* 多地址：单证取址的依据。发票抬头、收货人、通知方常常是三家不同的公司，
          所以它与上面那条"主地址"不是重复，而是"按用途各取所需"。 */}
      {result?.id ? <PartnerAddressPanel partnerId={result.id} /> : null}

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

      {result && (
        <PartnerOwnerModal
          open={ownerOpen}
          partner={result}
          onClose={() => setOwnerOpen(false)}
          onChanged={() => {
            // 转移之后这一页显示的负责人会变，重取一次详情最省事。
            void query.refetch()
          }}
        />
      )}
    </Show>
  )
}
