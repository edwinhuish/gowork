import { Alert, Divider, Form, Input } from 'antd'
import { useOne } from '@refinedev/core'
import { useForm } from '@refinedev/antd'
import { useParams } from 'react-router'

import { PageForm } from '@/components/page'
import type { Shipment } from '@/types'

import { AllocationEditor } from './allocation-editor'
import { ShipmentFormFields } from './form-fields'

/**
 * 编辑出货单。只有草稿可改：已确认的一票，仓库与货代已经在按它备货，
 * 改数量等于让他们白干。后端同样会拒（409），这里的禁用只是提前说明。
 */
export function ShipmentEdit() {
  const { id } = useParams<{ id: string }>()
  const { formProps, saveButtonProps, form } = useForm<Shipment>({
    resource: 'shipments',
    action: 'edit',
    redirect: 'show',
  })
  // 与 useForm 共用同一份 getOne 缓存，不会多打一次接口。
  const { query: detailQuery } = useOne<Shipment>({ resource: 'shipments', id })
  const shipment = detailQuery.data?.data

  const status = Form.useWatch<number | undefined>('status', form)
  const locked = status !== undefined && status !== 10
  const shipmentId = Number(id)

  return (
    <PageForm
      action="edit"
      title="编辑出货单"
      formProps={formProps}
      saveButtonProps={{ ...saveButtonProps, disabled: locked }}
      notice={
        locked ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="这张出货单不是草稿，不能再改"
            description="已确认的一票，仓库与货代已经在按它备货。要作废请改为已取消，数量会还给对应的订单行。"
          />
        ) : null
      }
      wide
    >

      {/* 乐观锁：把打开页面时读到的版本原样带回服务端，别人先改过就会拿到 409。 */}

      <Form.Item name="updated_at" hidden>
        <Input />

      </Form.Item>
      <ShipmentFormFields />

      <Divider orientation="left" style={{ margin: '8px 0 16px' }}>
        出货明细
      </Divider>

      <AllocationEditor
        excludeShipmentId={Number.isFinite(shipmentId) ? shipmentId : undefined}
        existing={shipment?.allocations}
      />
    </PageForm>
  )
}
