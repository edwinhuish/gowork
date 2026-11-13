import { useEffect, useMemo, useState } from 'react'
import { useInvalidate, useNavigation, useSelect } from '@refinedev/core'
import { Alert, Checkbox, Modal, Select, Space, message } from 'antd'

import { partnerApi } from '@/api/partners'
import type { Partner, PartnerDuplicateCandidate } from '@/types'

/**
 * 把当前主体并入另一条。
 *
 * 方向固定为「当前这条被并走、选中的那条留下」：用户是从"多余的那条"的详情页点进来的，
 * 这与他脑子里的方向一致；而接口里留下的是路径上的 :id，所以调用时两个 id 换位。
 *
 * 合并不可逆，所以这里有三道闸：一段说清会动什么的说明、必须明确勾选的确认、
 * 以及执行后跳到留下的那条（不留在一条即将消失的页面上）。
 */
export function PartnerMergeModal({
  open,
  partner,
  resource,
  onClose,
}: {
  open: boolean
  partner: Partner
  /** 当前页面所属资源，合并成功后跳到它的详情页。 */
  resource: 'customers' | 'suppliers' | 'forwarders'
  onClose: () => void
}) {
  const { show } = useNavigation()
  const invalidate = useInvalidate()
  const [targetId, setTargetId] = useState<number | undefined>(undefined)
  const [confirmed, setConfirmed] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [candidates, setCandidates] = useState<PartnerDuplicateCandidate[]>([])

  const { options: allOptions } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  // 打开时就查一次"可能是同一家"：多数合并正是冲着刚发现的那条重复去的，
  // 让人再从头翻一遍全部客户没有必要。
  useEffect(() => {
    if (!open) return
    let cancelled = false
    partnerApi
      .duplicateCandidates({
        name: partner.name,
        country: partner.country,
        phone: partner.phone,
        email: partner.email,
        exclude_id: partner.id,
      })
      .then((hits) => {
        if (!cancelled) setCandidates(hits)
      })
      .catch(() => {
        // 候选拿不到不影响手工选：下面的下拉里仍然能搜到全部主体。
        if (!cancelled) setCandidates([])
      })
    return () => {
      cancelled = true
    }
  }, [open, partner.id, partner.name, partner.country, partner.phone, partner.email])

  // 候选排最前并带上命中原因，其余主体照常可选——也可能要并进一条查重查不出来
  // 但人知道是同一家的记录（名称拼错到系统性认不出来）。
  const options = useMemo(() => {
    const picked = candidates.map((item) => ({
      value: item.partner.id,
      label: `${item.partner.code} · ${item.partner.name}（${item.reasons.join('；')}）`,
    }))
    const taken = new Set(picked.map((item) => item.value))
    const rest = allOptions.filter(
      (item) => Number(item.value) !== partner.id && !taken.has(Number(item.value)),
    )
    return [...picked, ...rest]
  }, [allOptions, candidates, partner.id])

  const submit = async () => {
    if (!targetId) {
      message.warning('请选择要并进去的那条主体')
      return
    }
    setSubmitting(true)
    try {
      const res = await partnerApi.merge(targetId, partner.id, partner.updated_at)
      const total = Object.values(res.moved).reduce((sum, n) => sum + n, 0)
      message.success(`已把 ${res.absorbed.code} 并入 ${res.partner.code}，迁移 ${total} 条引用`)
      onClose()
      await invalidate({ resource, invalidates: ['list'] })
      await invalidate({ resource: 'partners', invalidates: ['list'] })
      show(resource, res.partner.id)
    } catch (err) {
      // 409 是常态：货源冲突、或这条主体刚被别人改过，后端都把原因写清楚了。
      message.error(err instanceof Error ? err.message : '合并失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      open={open}
      title={`合并主体 ${partner.code}`}
      okText="确认合并"
      cancelText="返回"
      okButtonProps={{ disabled: !confirmed || !targetId }}
      confirmLoading={submitting}
      onOk={() => void submit()}
      onCancel={onClose}
      destroyOnHidden
    >
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 16 }}
        message="合并不可撤销"
        description={
          <>
            <div>
              当前这条 <b>{partner.code} · {partner.name}</b> 会被并入下面选中的那条，之后从列表里消失
              （编码可以重新使用，记录本身仍能从「含已删除」里恢复）。
            </div>
            <div style={{ marginTop: 6 }}>
              它的联系人、跟进、线索、商机、报价、订单、采购单、合同、应收应付、收付款、
              运杂费、出入库单、产品货源与附件都会迁到留下的那条；角色取并集，
              留下的那条空着的字段会用这边的值补上。
            </div>
          </>
        }
      />

      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Select
          style={{ width: '100%' }}
          placeholder="选一条留下（可搜编码或名称）"
          showSearch
          value={targetId}
          onChange={(value: number) => setTargetId(value)}
          options={options}
          filterOption={(input, option) =>
            String(option?.label ?? '')
              .toLowerCase()
              .includes(input.toLowerCase())
          }
        />
        <Checkbox checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)}>
          我确认这两条是同一家，并把「{partner.code}」并过去
        </Checkbox>
      </Space>
    </Modal>
  )
}
