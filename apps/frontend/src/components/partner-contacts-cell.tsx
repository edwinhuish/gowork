import { Tooltip } from '@/components/ui/Tooltip'


import type { Contact } from '@/types'
import { Empty } from './fields'
import { PartnerAvatar } from './partner-avatar'

/**
 * 往来单位列表里的多联系人头像组。
 *
 * 后端把一个主体名下的全部联系人整页回填到 `partner.contacts`，顺序是
 * 主联系人排第一。这里只负责展示：**最多 3 个**，其余收进 `+N`。
 *
 * 为什么收起而不是让单元格横向溢出：一列在表格里宽度固定，十个人排开会把
 * 「负责人」「账期」挤到看不见，而列表的主要用途是扫读，不是看全每个人。
 * 完整的名单在详情页与 /contacts，`+N` 与每个头像的悬浮提示都能看到是谁。
 *
 * 联系人没有独立的头像字段，占位标识取姓名首字（与主体 Logo 同一套调色板，
 * 但哈希的是人名）：同一个人在任何一页都是同一个颜色，便于认人。
 */
export const PARTNER_CONTACTS_MAX = 3

interface PartnerContactsCellProps {
  contacts?: Contact[]
  /** 最多显示几个，超出收进 +N。默认 3，三个以上在 240px 宽的列里就挤了。 */
  max?: number
}

export function PartnerContactsCell({
  contacts,
  max = PARTNER_CONTACTS_MAX,
}: PartnerContactsCellProps) {
  const rows = contacts ?? []
  if (rows.length === 0) return <Empty />

  const visible = rows.slice(0, max)
  const rest = rows.slice(max)

  return (
    <span className="partner-contacts" aria-label={`${rows.length} 位联系人`}>
      {visible.map((contact) => (
        <Tooltip key={contact.id} title={<ContactTip contact={contact} />}>
          <span className="partner-contacts-item">
            <PartnerAvatar name={contact.name} size={28} />
            {/* 主联系人加一个角标：顺序已经在后端保证，但只靠位置说不清，
                翻页或筛选之后用户没有参照物来判断哪个才是主联系人。 */}
            {contact.is_primary ? (
              <span className="partner-contacts-primary" aria-label="主联系人" />
            ) : null}
          </span>
        </Tooltip>
      ))}
      {rest.length > 0 ? (
        <Tooltip title={rest.map((contact) => contact.name).join('、')}>
          <span
            className="partner-contacts-more"
            role="img"
            aria-label={`另有 ${rest.length} 位联系人`}
          >
            +{rest.length}
          </span>
        </Tooltip>
      ) : null}
    </span>
  )
}

/** 悬浮提示里的正文：姓名 + 职位 + 电话 / 邮箱，空字段不占行。 */
function ContactTip({ contact }: { contact: Contact }) {
  return (
    <span className="partner-contacts-tip">
      <strong>{contact.name}</strong>
      {contact.is_primary ? <span className="partner-contacts-tip-tag">主联系人</span> : null}
      {contact.title ? <span>{contact.title}</span> : null}
      {contact.phone ? <span>{contact.phone}</span> : null}
      {contact.email ? <span>{contact.email}</span> : null}
    </span>
  )
}
