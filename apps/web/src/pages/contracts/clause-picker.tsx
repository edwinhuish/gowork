import { useQuery } from '@tanstack/react-query'
import { Form, Select, Tag } from 'antd'

import { http } from '@/api/client'
import type { Clause, PageResult } from '@/types'

/**
 * 从条款库挑条款（T-27）。
 *
 * 只提交模板 id（`clause_ids`），正文由服务端抄成**快照**存进这份合同——
 * 前端把正文也传上去的话，"之后改模板不影响这份合同"就没有保证了。
 *
 * 顺序即合同上的顺序：多选框里的先后就是提交顺序，后端按它编号。
 */
export function ContractClausePicker() {
  const { data, isLoading } = useQuery({
    // 只取启用的模板：停用的模板后端也会拒（400），这里不把它们摆出来。
    queryKey: ['clauses', 'active'],
    queryFn: () => http.get<PageResult<Clause>>('/clauses?page_size=100'),
  })

  return (
    <Form.Item
      name="clause_ids"
      label="合同条款"
      className="form-item-full"
      extra="选中后会抄成快照存进这份合同；之后改条款库的模板，不影响已经生成的合同"
    >
      <Select
        mode="multiple"
        allowClear
        loading={isLoading}
        placeholder="从条款库选择（可多选，顺序即合同上的顺序）"
        optionFilterProp="label"
        options={(data?.list ?? []).map((c) => ({
          value: c.id,
          label: `${c.code} · ${c.title}`,
        }))}
        tagRender={({ label, closable, onClose }) => (
          <Tag closable={closable} onClose={onClose} style={{ marginInlineEnd: 4 }}>
            {label}
          </Tag>
        )}
      />
    </Form.Item>
  )
}
