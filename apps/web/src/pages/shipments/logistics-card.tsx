import { useEffect, useState } from 'react'
import { useInvalidate } from '@refinedev/core'
import { Button, Card, Col, Form, Input, message, Row, Typography } from 'antd'

import { shipmentApi, type ShipmentLogisticsInput } from '@/api/shipments'
import type { Shipment } from '@/types'

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/

/**
 * 物流节点：提单号、船名航次、ETD / ETA。
 *
 * 就地编辑而不跳编辑页：出货单只有草稿能改，而这一组值是订舱之后才有的，
 * 那时单子早已确认。为记一个提单号去把已确认的单据解锁，是拿数据完整性
 * 换一次点击。后端也给了独立入口（PUT /shipments/:id/logistics）。
 *
 * 日期用文本框而不是 DatePicker：项目不引入 dayjs 之外的日期依赖，
 * 而 DatePicker 要在 dayjs 与字符串之间来回转，多一处出错的地方。
 */
export function LogisticsCard({ shipment }: { shipment: Shipment }) {
  const [form] = Form.useForm<ShipmentLogisticsInput>()
  const [saving, setSaving] = useState(false)
  const invalidate = useInvalidate()

  // 后端返回的时间带时分秒（2026-10-31T00:00:00Z），输入框只要日期那一段。
  useEffect(() => {
    form.setFieldsValue({
      bl_no: shipment.bl_no ?? '',
      vessel: shipment.vessel ?? '',
      etd: shipment.etd ? shipment.etd.slice(0, 10) : '',
      eta: shipment.eta ? shipment.eta.slice(0, 10) : '',
    })
  }, [shipment, form])

  const save = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      await shipmentApi.updateLogistics(shipment.id, {
        bl_no: values.bl_no?.trim() ?? '',
        vessel: values.vessel?.trim() ?? '',
        etd: values.etd?.trim() || undefined,
        eta: values.eta?.trim() || undefined,
      })
      message.success('物流节点已保存')
      await invalidate({ resource: 'shipments', invalidates: ['detail'], id: shipment.id })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const dateRule = (label: string) => ({
    validator: (_: unknown, value?: string) => {
      const v = value?.trim()
      if (!v || DATE_PATTERN.test(v)) return Promise.resolve()
      return Promise.reject(new Error(`${label}格式应为 YYYY-MM-DD`))
    },
  })

  return (
    <Card title="物流节点" style={{ marginTop: 16 }} extra={<Button type="primary" loading={saving} onClick={save}>保存</Button>}>
      <Typography.Paragraph type="secondary" style={{ marginTop: -4 }}>
        提单号与船期是订舱之后才有的，任何状态都能改，不影响出货单本身。
        与出货单的编辑不同：那里改的是数量，动一下仓库与货代手里的单就白做了。
      </Typography.Paragraph>

      <Form form={form} layout="vertical">
        <Row gutter={12}>
          <Col xs={24} md={8}>
            <Form.Item name="bl_no" label="提单号" rules={[{ max: 64, message: '最多 64 个字符' }]}>
              <Input placeholder="COSU1234567890" />
            </Form.Item>
          </Col>
          <Col xs={24} md={8}>
            <Form.Item name="vessel" label="船名航次" rules={[{ max: 128, message: '最多 128 个字符' }]}>
              <Input placeholder="COSCO SHIPPING / 028E" />
            </Form.Item>
          </Col>
          <Col xs={12} md={4}>
            <Form.Item name="etd" label="ETD（开船）" rules={[dateRule('ETD')]}>
              <Input placeholder="YYYY-MM-DD" />
            </Form.Item>
          </Col>
          <Col xs={12} md={4}>
            <Form.Item name="eta" label="ETA（到港）" rules={[dateRule('ETA')]}>
              <Input placeholder="YYYY-MM-DD" />
            </Form.Item>
          </Col>
        </Row>
      </Form>
    </Card>
  )
}
