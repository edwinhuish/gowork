import { useQuery } from '@tanstack/react-query'
import { Alert } from 'antd'

import { creditApi } from '@/api/credit'
import { Money } from '@/components/fields'

/**
 * 下单时的客户额度提示。
 *
 * **只提示，不拦**：草稿不占额度，真正的拦截在确认那一步。把数字提前摆出来，
 * 是为了让人先知道差多少，而不是录完明细、点了确认才被 409 打回来。
 */
export function CreditHint({ partnerId }: { partnerId?: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['partner-credit', partnerId],
    queryFn: () => creditApi.summary(partnerId as number),
    enabled: Boolean(partnerId),
  })

  if (!partnerId || isLoading || !data) return null

  if (data.unlimited) {
    return (
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="该客户没有设置信用额度（视为不限）"
        description={
          <span>
            当前敞口 <Money value={data.exposure} />；设了额度之后，超额订单会在确认时被拦下。
          </span>
        }
      />
    )
  }

  const over = data.available < 0

  return (
    <Alert
      type={over ? 'warning' : 'success'}
      showIcon
      style={{ marginBottom: 16 }}
      message={
        <span>
          信用额度 <Money value={data.credit_limit} />，已用 <Money value={data.exposure} />，
          {over ? '已超 ' : '可用 '}
          <Money value={over ? -data.available : data.available} />
        </span>
      }
      description={
        over
          ? '客户已超出额度：确认新订单需要管理员强制放行。'
          : `已用 = 未结清应收 ${data.open_receivable.toFixed(2)} + 未开票订单 ${data.unbilled_order.toFixed(2)}`
      }
    />
  )
}
