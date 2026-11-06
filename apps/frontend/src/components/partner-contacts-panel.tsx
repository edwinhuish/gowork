import { Card, Space } from 'antd'
import { Link } from 'react-router'

import { AccentTag, Empty, Mono } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import type { Contact } from '@/types'

/**
 * 主体详情页的联系人名单。
 *
 * 列表里的联系人列只放前三个头像（列宽固定，铺开会把别的列挤走），
 * 而主体详情是「这家客户都有谁」最该回答清楚的地方 —— 报价发给谁、
 * 单证上的通知方是谁、催款找谁，都从这里看。
 *
 * 数据直接用主体详情里回填的 contacts：后端已经按主联系人优先排好，
 * 这里不再单独拉一次 /contacts，否则两处顺序迟早不一致。
 */
export function PartnerContactsPanel({ contacts }: { contacts?: Contact[] }) {
  const rows = contacts ?? []

  return (
    <Card
      title="联系人"
      size="small"
      style={{ marginTop: 16 }}
      extra={<Link to="/contacts">全部联系人</Link>}
    >
      <ResponsiveTable<Contact>
        rowKey="id"
        size="small"
        pagination={false}
        rows={rows}
        empty="还没有联系人。报价与单证上的联系人取自这里，第一位是主联系人。"
        columns={[
          {
            title: '姓名',
            dataIndex: 'name',
            render: (_: string, record) => (
              <Space size={6}>
                <span>{record.name}</span>
                {record.is_primary ? <AccentTag>主联系人</AccentTag> : null}
              </Space>
            ),
          },
          {
            title: '职位',
            dataIndex: 'title',
            width: 130,
            render: (value: string) => value || <Empty />,
          },
          {
            title: '电话',
            dataIndex: 'phone',
            width: 170,
            render: (value: string) => (value ? <Mono>{value}</Mono> : <Empty />),
          },
          {
            title: '邮箱',
            dataIndex: 'email',
            render: (value: string) => value || <Empty />,
          },
          {
            title: '语言 / 渠道',
            dataIndex: 'channel',
            width: 150,
            // 两件事合成一列：它们都决定「怎么联系上这个人」，
            // 拆成两列在窄屏会把姓名挤出可见区。
            render: (_: string, record) => {
              const text = [record.language, record.channel].filter(Boolean).join(' / ')
              return text || <Empty />
            },
          },
          {
            title: '操作',
            width: 80,
            render: (_: unknown, record) =>
              record.deleted_at ? (
                <span className="cell-sub">已删除</span>
              ) : (
                <Link to={`/contacts/edit/${record.id}`}>编辑</Link>
              ),
          },
        ]}
      />

      {/* 新增走 /contacts 的新建页：那里有主体下拉，而这里点进来的人
          已经知道自己要加到哪家 —— 不为了省一次选择把表单嵌进详情页。
          不带 partner_id 预筛：联系人列表的筛选表单读的是自己的字段，
          URL 上的裸参数只会筛了数据却不显示条件，看着像丢了行。 */}
      <div className="cell-sub" style={{ marginTop: 8 }}>
        新增联系人与改主联系人请到
        <Link to="/contacts">联系人列表</Link>
        。
      </div>
    </Card>
  )
}
