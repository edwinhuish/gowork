import { useEffect, useMemo, useState } from 'react'
import { Button, Col, Form, Input, InputNumber, Row, Select, Typography } from 'antd'

import { shipmentApi } from '@/api/shipments'
import type { ShipmentAllocation, ShipmentCandidate } from '@/types'
import { formatQty } from '@/utils/format'

export interface AllocationRow {
  sales_order_item_id?: number
  qty?: number
  remark?: string
}

interface RowInfo {
  docNo: string
  skuCode: string
  nameCN: string
  spec: string
  unit: string
  /** 已出多少。 */
  shipped?: number
  /** 还能出多少；编辑时后端把本票占的数量还了回来，所以这里是含本行的上限。 */
  unshipped?: number
}



/**
 * 出货分配编辑器：选销售订单**行**，各填多少数量。
 *
 * 不能按订单头分配：一张订单里两行可能是同一产品的不同包装，
 * 按订单头就分不清出的是哪一种。
 *
 * 「还能出多少」由后端算（订单行数量 − 已出数量），这里只显示与当作输入上限——
 * 前端限不住两个页面同时开着的情况，最终判定在服务端。
 */
export function AllocationEditor({
  excludeShipmentId,
  existing,
}: {
  /** 编辑页传当前这一票，让候选里包含本票已占的行。 */
  excludeShipmentId?: number
  /** 编辑页的已有分配行，用于候选里找不到时兜底显示快照。 */
  existing?: ShipmentAllocation[]
}) {
  const form = Form.useFormInstance()
  const rows = Form.useWatch<AllocationRow[]>('allocations', form) ?? []

  const [candidates, setCandidates] = useState<ShipmentCandidate[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>()
  const [keyword, setKeyword] = useState('')
  const [picked, setPicked] = useState<number>()

  // 关键字输入后等一下再发请求：每敲一个字母打一次接口，选行下拉会一直闪。
  useEffect(() => {
    let alive = true
    const timer = setTimeout(() => {
      setLoading(true)
      shipmentApi
        .candidates(keyword, excludeShipmentId)
        .then((list) => {
          if (!alive) return
          setCandidates(list)
          setError(undefined)
        })
        .catch((err: unknown) => {
          if (!alive) return
          setCandidates([])
          setError(err instanceof Error ? err.message : '可出货订单行加载失败')
        })
        .finally(() => {
          if (alive) setLoading(false)
        })
    }, 250)
    return () => {
      alive = false
      clearTimeout(timer)
    }
  }, [keyword, excludeShipmentId])

  const infoById = useMemo(() => {
    const map = new Map<number, RowInfo>()
    for (const c of candidates) {
      map.set(c.sales_order_item_id, {
        docNo: c.sales_order_doc_no,
        skuCode: c.sku_code,
        nameCN: c.name_cn,
        spec: c.spec,
        unit: c.unit,
        shipped: c.shipped_qty,
        unshipped: c.unshipped_qty,
      })
    }
    // 候选里没有的行（订单已取消 / 已出完且不是本票）仍要能显示，
    // 否则编辑一张老单时，行内容会变成一串数字 ID。
    for (const a of existing ?? []) {
      if (map.has(a.sales_order_item_id)) continue
      map.set(a.sales_order_item_id, {
        docNo: a.sales_order_doc_no,
        skuCode: a.sku_code,
        nameCN: a.name_cn,
        spec: a.spec,
        unit: a.unit,
      })
    }
    return map
  }, [candidates, existing])

  const usedIds = new Set(
    rows.map((r) => r.sales_order_item_id).filter((id): id is number => typeof id === 'number'),
  )
  const options = candidates
    .filter((c) => !usedIds.has(c.sales_order_item_id))
    .map((c) => ({
      value: c.sales_order_item_id,
      label: `${c.sales_order_doc_no} · ${c.sku_code} ${c.name_cn}（还能出 ${c.unshipped_qty}）`,
    }))

  const spans = { item: 9, remain: 4, qty: 4, remark: 5, action: 2 }

  return (
    <div>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
        一票可以装多张订单的货：分别选行、分别填数量。数量不得超过该订单行的未出数量，超出由后端拒绝。
      </Typography.Paragraph>

      <Row gutter={8} style={{ marginBottom: 4 }}>
        <Col span={spans.item}>
          <Typography.Text type="secondary">销售订单行</Typography.Text>
        </Col>
        <Col span={spans.remain}>
          <Typography.Text type="secondary">已出 / 未出</Typography.Text>
        </Col>
        <Col span={spans.qty}>
          <Typography.Text type="secondary">本次出货</Typography.Text>
        </Col>
        <Col span={spans.remark}>
          <Typography.Text type="secondary">备注</Typography.Text>
        </Col>
        <Col span={spans.action} />
      </Row>

      <Form.List
        name="allocations"
        rules={[
          {
            // 空的一票出货没有意义，后端也会拒（400）。在这里先拦，
            // 是因为"点了保存才报错"看起来像系统坏了。
            validator: async (_, value: unknown[]) => {
              if (!value || value.length === 0) {
                throw new Error('至少加一行：这一票出哪条订单行、出多少')
              }
            },
          },
        ]}
      >
        {(fields, { add, remove }, { errors }) => (
          <>
            {fields.map((field) => {
              const row = rows[field.name] ?? {}
              const info = row.sales_order_item_id ? infoById.get(row.sales_order_item_id) : undefined
              const remain =
                info?.unshipped === undefined
                  ? undefined
                  : Math.max(info.unshipped - (row.qty ?? 0), 0)

              return (
                <Row gutter={8} key={field.key} align="top">
                  <Col span={spans.item}>
                    {/* 隐藏字段必须渲染，否则提交时这一行不知道对应哪条订单 */}
                    <Form.Item
                      name={[field.name, 'sales_order_item_id']}
                      rules={[{ required: true, message: '请选择销售订单行' }]}
                      hidden
                    >
                      <Input />
                    </Form.Item>
                    <div style={{ paddingTop: 6 }} className="cell-stack">
                      <span className="cell-strong">
                        {info ? `${info.skuCode} ${info.nameCN}` : '订单行 #'}
                        {!info && row.sales_order_item_id}
                      </span>
                      <span className="cell-sub">
                        {info?.docNo || '来源订单未知'}
                        {info?.spec ? ` · ${info.spec}` : ''}
                      </span>
                    </div>
                  </Col>

                  <Col span={spans.remain}>
                    <div style={{ paddingTop: 6 }}>
                      {info?.unshipped === undefined ? (
                        <Typography.Text type="secondary">—</Typography.Text>
                      ) : (
                        <span className="cell-mono">
                          已出 {formatQty(info.shipped ?? 0)} / 未出 {formatQty(remain ?? 0)}
                        </span>
                      )}
                    </div>
                  </Col>

                  <Col span={spans.qty}>
                    <Form.Item
                      name={[field.name, 'qty']}
                      rules={[{ required: true, message: '填数量' }]}
                    >
                      <InputNumber
                        style={{ width: '100%' }}
                        min={0.001}
                        step={1}
                        precision={3}
                        placeholder="0"
                      />
                    </Form.Item>
                  </Col>

                  <Col span={spans.remark}>
                    <Form.Item name={[field.name, 'remark']}>
                      <Input maxLength={255} placeholder="包装、批号等" />
                    </Form.Item>
                  </Col>

                  <Col span={spans.action}>
                    <Button type="link" danger onClick={() => remove(field.name)}>
                      删除
                    </Button>
                  </Col>
                </Row>
              )
            })}

            <Select
              style={{ width: '100%', marginTop: 4 }}
              placeholder="搜索订单号 / SKU / 品名，添加一行出货"
              showSearch
              value={picked}
              loading={loading}
              options={options}
              filterOption={false}
              onSearch={setKeyword}
              onChange={(itemId: number) => {
                const c = candidates.find((x) => x.sales_order_item_id === itemId)
                add({
                  sales_order_item_id: itemId,
                  // 默认出完：分批出货时改这个数就行，比从 0 开始填省事。
                  qty: c?.unshipped_qty ?? 1,
                  remark: '',
                })
                setPicked(undefined)
              }}
            />

            {error ? (
              <Typography.Text type="danger" style={{ display: 'block', marginTop: 8 }}>
                {error}
              </Typography.Text>
            ) : null}

            {options.length === 0 && !loading && !error ? (
              <Typography.Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
                没有可出货的订单行：订单要先确认，且不能已经出完。
              </Typography.Text>
            ) : null}

            <Form.ErrorList errors={errors} />
          </>
        )}
      </Form.List>
    </div>
  )
}
