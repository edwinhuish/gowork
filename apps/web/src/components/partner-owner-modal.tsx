import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Modal, Select, Space, Typography, message } from 'antd'

import { partnerApi } from '@/api/partners'
import { userApi } from '@/api/users'
import type { Partner } from '@/types'

/**
 * 把主体转给另一个业务员（T-32）。
 *
 * 只有管理员与主管看得到这个入口（接口对业务员返回 403）：
 * 转移是主管的动作，编辑是录单人日常——混在一起，后者也能顺手把客户划到自己名下。
 *
 * 选人列表只给**启用中**的账号：转给停用的人等于把客户交给一个看不到的人。
 */
export function PartnerOwnerModal({
  open,
  partner,
  onClose,
  onChanged,
}: {
  open: boolean
  partner: Partner
  onClose: () => void
  /** 保存成功后由调用方刷新详情。 */
  onChanged: () => void | Promise<void>
}) {
  const [ownerId, setOwnerId] = useState<number>(partner.owner_id ?? 0)
  const [saving, setSaving] = useState(false)

  const { data: users, isLoading, error } = useQuery({
    queryKey: ['users-active'],
    queryFn: () => userApi.listActive(),
    // 只在打开时查：这一页大部分时间不需要这份名单。
    enabled: open,
  })

  useEffect(() => {
    if (open) setOwnerId(partner.owner_id ?? 0)
  }, [open, partner.owner_id])

  const save = async () => {
    setSaving(true)
    try {
      await partnerApi.transferOwner(partner.id, ownerId, partner.updated_at)
      message.success(ownerId === 0 ? '已退回未分配（公共池）' : '已转移负责人')
      onClose()
      await onChanged()
    } catch (err) {
      // 409 在这里是常态：别人刚改过这条主体，后端把原因写在文案里。
      message.error(err instanceof Error ? err.message : '转移失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      open={open}
      title={`转移负责人 · ${partner.code}`}
      okText="保存"
      cancelText="返回"
      confirmLoading={saving}
      onOk={() => void save()}
      onCancel={onClose}
      destroyOnHidden
    >
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Select
          style={{ width: '100%' }}
          loading={isLoading}
          value={ownerId}
          onChange={setOwnerId}
          showSearch
          filterOption={(input, option) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
          options={[
            // 0 不是一个真实的账号，但"退回公共池"是一个真实的状态：
            // 业务员离职、客户暂时没人跟，都需要把归属清掉。
            { value: 0, label: '未分配（公共池，谁都能跟进）' },
            ...(users ?? []).map((u) => ({
              value: u.id,
              label: `${u.username}${u.nickname ? ` · ${u.nickname}` : ''}（${u.role}）`,
            })),
          ]}
        />
        <Typography.Text type="secondary">
          归属只记在客户上，这个客户的订单、报价、合同、应收与出货会一起跟着走——
          不会出现"订单归张三、客户归李四"。
        </Typography.Text>
      </Space>
    </Modal>
  )
}
