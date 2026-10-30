import { useState } from 'react'

import type { CrudFilters, BaseRecord } from '@refinedev/core'
import { useInvalidate } from '@refinedev/core'
import { useTable } from '@refinedev/antd'
import { Button, Checkbox, Form, Input, message, Select, Table, Tag } from 'antd'

import { AccentTag, Empty, Mono } from '@/components/fields'
import { PageList } from '@/components/page'
import { CURRENCY_STATUS_OPTIONS } from '@/constants/catalog'
import { http } from '@/api/client'
import type { Currency } from '@/types'

interface CurrencySearch {
  keyword?: string
  status?: number
}

/**
 * 币种列表。
 *
 * 「本币」单独一列而不是并进状态：本币不是"更重要的启用"，
 * 它是折算的基准，改了它所有新单据的本币金额口径都跟着变。
 */
export function CurrencyList() {
  const [showDeleted, setShowDeleted] = useState(false)
  const { tableProps, searchFormProps, setFilters } = useTable<Currency>({
    resource: 'currencies',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as CurrencySearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.status !== undefined && v.status !== null) {
        filters.push({ field: 'status', operator: 'eq', value: v.status })
      }
      return filters
    },
  })

  // 归档不等于删除：归档的币种平时就列着，只有删掉的才藏在这个开关后面。
  //
  // 在**事件里**发而不是 useEffect 里：refine 的 setFilters 依赖
  // filters.permanent 这个数组，哪天这页加上 permanent（内联字面量，每次渲染
  // 都是新数组），setFilters 就每次都是新引用 —— 放进 effect 依赖里会自激成
  // "Maximum update depth exceeded"（产品货源面板实测踩过）。开关是事件。
  const toggleDeleted = (checked: boolean) => {
    setShowDeleted(checked)
    setFilters(
      checked ? [{ field: 'include_deleted', operator: 'eq', value: true }] : [],
      'replace',
    )
  }

  const invalidate = useInvalidate()
  const restore = async (id: number) => {
    try {
      await http.put<BaseRecord>(`/currencies/${id}/restore`)
      await invalidate({ resource: 'currencies', invalidates: ['list'] })
    } catch (err) {
      // 代码被别的币种占了是 409，后端文案写明了是哪个代码。
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <PageList
      createLabel="新建币种"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索代码 / 名称" allowClear />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="全部归档状态" allowClear options={CURRENCY_STATUS_OPTIONS} />
          </Form.Item>
        </>
      }
      extra={
        <Checkbox checked={showDeleted} onChange={(e) => toggleDeleted(e.target.checked)}>
          含已删除
        </Checkbox>
      }
    >
      <Table.Column
        dataIndex="code"
        title="代码"
        width={100}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      <Table.Column dataIndex="name" title="名称" width={160} />
      <Table.Column
        dataIndex="symbol"
        title="符号"
        width={80}
        render={(value: string) => value || <Empty />}
      />
      <Table.Column dataIndex="decimal_places" title="小数位" width={90} align="right" />
      <Table.Column
        dataIndex="is_base"
        title="本币"
        width={90}
        render={(value: boolean) => (value ? <AccentTag>本币</AccentTag> : <Empty />)}
      />
      <Table.Column
        dataIndex="status"
        title="归档"
        width={90}
        render={(value: number) =>
          value === 1 ? <Tag color="success">在用</Tag> : <Tag>已归档</Tag>
        }
      />
      <Table.Column
        dataIndex="remark"
        title="备注"
        render={(value: string) => value || <Empty />}
      />
      <Table.Column
        title="操作"
        width={90}
        render={(_, record: Currency) =>
          // 归档的人看得见也想得到，删掉的人才需要这一列。
          record.deleted_at ? (
            <Button size="small" type="link" onClick={() => restore(record.id)}>
              恢复
            </Button>
          ) : null
        }
      />
    </PageList>
  )
}
