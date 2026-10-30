import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Input, Popconfirm, Select, Space, Table, Tag, Typography, message } from 'antd'

import { productCodeApi } from '@/api/product-codes'
import { Empty, Mono } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import { PRODUCT_CODE_SOURCE_OPTIONS, productCodeSourceLabel } from '@/constants/catalog'
import type { ProductCode } from '@/types'
import { formatTime } from '@/utils/format'

/**
 * 产品详情页的编号面板：SKU 之外那些「别人怎么叫它」的号。
 *
 * 不单独占一个菜单：编号的唯一用途是"客户报一个号，能搜回这件货"，
 * 离开产品语境它就是一张谁也不看的清单。
 *
 * 录入用行内表单而不是弹窗：一次要补三五个号（客户号、供应商号、工厂号），
 * 每个都弹一次窗比敲键盘还慢。
 */
export function CodePanel({ productId }: { productId: number }) {
  const [list, setList] = useState<ProductCode[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [code, setCode] = useState('')
  const [source, setSource] = useState<string>('customer')
  const [remark, setRemark] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setList(await productCodeApi.list(productId))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '编号列表加载失败')
    } finally {
      setLoading(false)
    }
  }, [productId])

  useEffect(() => {
    void load()
  }, [load])

  const add = async () => {
    const value = code.trim()
    if (!value) {
      message.warning('先填一个编号')
      return
    }
    setSaving(true)
    try {
      await productCodeApi.create({
        product_id: productId,
        code: value,
        source,
        remark: remark.trim(),
      })
      message.success(`已添加 ${value}`)
      setCode('')
      setRemark('')
      await load()
    } catch (err) {
      // 重复编号是 409，后端文案已经说清是哪个号，直接透出。
      message.error(err instanceof Error ? err.message : '添加失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (record: ProductCode) => {
    try {
      await productCodeApi.remove(record.id)
      message.success('已删除编号')
      await load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card title="其他编号" style={{ marginTop: 16 }}>
      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        客户物料号、供应商货号、工厂号，或改过的旧 SKU 都记在这里。产品列表的关键词搜索会一起搜它们
        —— 客户报一个旧号也能找到这件货。大小写与连字符不区分；改 SKU 时旧号自动沉淀进来。
      </Typography.Paragraph>

      <Space.Compact style={{ width: '100%', marginBottom: 12 }}>
        <Input
          placeholder="编号，如 ABC-123"
          value={code}
          maxLength={64}
          onChange={(e) => setCode(e.target.value)}
          onPressEnter={() => void add()}
          style={{ maxWidth: 260 }}
        />
        <Select
          value={source}
          onChange={setSource}
          options={PRODUCT_CODE_SOURCE_OPTIONS}
          style={{ width: 150 }}
        />
        <Input
          placeholder="备注（谁给的、什么场合），选填"
          value={remark}
          maxLength={255}
          onChange={(e) => setRemark(e.target.value)}
          onPressEnter={() => void add()}
        />
        <Button type="primary" loading={saving} onClick={() => void add()}>
          添加
        </Button>
      </Space.Compact>

      <ResponsiveTable
        rows={list}
        loading={loading}
        rowKey="id"
        size="small"
        pagination={false}
        empty="还没有其他编号。客户或供应商给过别的叫法，就记在这里。"
      >
        <Table.Column
          dataIndex="code"
          title="编号"
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="source"
          title="来源"
          width={140}
          render={(value: string) => <Tag>{productCodeSourceLabel(value) ?? '其它'}</Tag>}
        />
        <Table.Column
          dataIndex="remark"
          title="备注"
          width={220}
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          dataIndex="created_at"
          title="录入时间"
          width={170}
          render={(value: string) => formatTime(value)}
        />
        <Table.Column
          title="操作"
          width={90}
          render={(_, record: ProductCode) => (
            <Popconfirm
              title="删除这个编号？"
              description="删掉之后按它搜不到这件货了；要改写法就删掉重录。"
              okText="删除"
              cancelText="取消"
              onConfirm={() => remove(record)}
            >
              <Button size="small" type="link" danger>
                删除
              </Button>
            </Popconfirm>
          )}
        />
      </ResponsiveTable>
    </Card>
  )
}
