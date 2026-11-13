import { useEffect, useState } from 'react'
import { Alert, Form } from 'antd'

import { partnerApi } from '@/api/partners'
import type { PartnerDuplicateCandidate } from '@/types'

/** 输入停顿多久才查一次：每敲一个字都打接口，既浪费也会让提示一直闪。 */
const DEBOUNCE_MS = 500

/**
 * 往来单位表单的「可能已存在同一家」提示。
 *
 * 客户、供应商与货代共用这张表，也共用这三个模块的表单，所以查重提示只写这一处。
 * **只提示不阻断**：外贸同名公司在不同国家确实存在（两家 "Sunrise Trading"），
 * 拦住会让人无法把一笔真实询盘落库；而重复录入的代价是一份客户分成两半——
 * 跟进、商机、订单各挂一边，往后每一处聚合都要人工拼。两边都不该由系统替人决定。
 *
 * 编辑时自动排除自身：记录 id 在 initialValues 里，用 useWatch 读得到。
 */
export function PartnerDuplicateHint() {
  const form = Form.useFormInstance()
  const id = Form.useWatch<number | undefined>('id', form)
  const name = Form.useWatch<string | undefined>('name', form)
  const country = Form.useWatch<string | undefined>('country', form)
  const phone = Form.useWatch<string | undefined>('phone', form)
  const email = Form.useWatch<string | undefined>('email', form)
  const [candidates, setCandidates] = useState<PartnerDuplicateCandidate[]>([])

  useEffect(() => {
    const probe = {
      name: name?.trim() ?? '',
      country: country?.trim() ?? '',
      phone: phone?.trim() ?? '',
      email: email?.trim() ?? '',
    }
    // 名称、电话、邮箱全空时本地就退出：没有可比的输入，问了也是空。
    if (!probe.name && !probe.phone && !probe.email) {
      setCandidates([])
      return
    }

    // cancelled 挡的是"后发的请求先回来"：不挡的话，改完名字看到的是上一个名字的候选。
    let cancelled = false
    const timer = setTimeout(() => {
      partnerApi
        .duplicateCandidates({ ...probe, exclude_id: id })
        .then((hits) => {
          if (!cancelled) setCandidates(hits)
        })
        .catch(() => {
          // 提示查不到不该挡住录入：静默清空，保存时编码唯一索引仍然兜底。
          if (!cancelled) setCandidates([])
        })
    }, DEBOUNCE_MS)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [id, name, country, phone, email])

  if (candidates.length === 0) return null

  return (
    <Alert
      type="warning"
      showIcon
      className="form-item-full"
      style={{ marginBottom: 16 }}
      message="可能已存在同一家主体，确认不是重复再保存"
      description={
        <ul style={{ margin: 0, paddingLeft: 18 }}>
          {candidates.map((item) => (
            <li key={item.partner.id}>
              <span className="cell-mono">{item.partner.code}</span> {item.partner.name}
              <span className="cell-sub">（{item.reasons.join('；')}）</span>
            </li>
          ))}
        </ul>
      }
    />
  )
}
