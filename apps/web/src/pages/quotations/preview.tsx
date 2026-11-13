import { useState } from 'react'
import type { FormInstance } from 'antd'
import { Alert, message } from 'antd'

import { quotationApi } from '@/api/quotations'
import type { QuotationPreview } from '@/types'
import { formatPercent } from '@/utils/format'
import type { LineItemRow } from '@/components/line-items-editor'

function money(value: number | string): string {
  return value.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

/**
 * 报价试算。
 *
 * 走独立接口而不是在前端算：成本要从货源取、汇率要看当天，前端算出来的毛利
 * 与保存后的数字一旦不同，用户就再也不信这个数了。同一个 buildItems 出两遍结果，
 * 口径不可能分叉。
 */
export function useQuotationPreview(form: FormInstance) {
  const [preview, setPreview] = useState<QuotationPreview | null>(null)
  const [loading, setLoading] = useState(false)

  const reset = () => setPreview(null)

  const run = async () => {
    let values: { currency_code?: string; items?: LineItemRow[] }
    try {
      values = (await form.validateFields()) as typeof values
    } catch {
      // 校验失败时 antd 已经把字段标红，这里不需要再弹一次。
      return
    }

    const items = (values.items ?? [])
      // 数量是定点字符串：这里的"有没有填"只决定这一行要不要送去试算。
      .filter((it) => it?.product_id && Number(it?.qty ?? 0) > 0)
      .map((it) => ({
        product_id: it.product_id as number,
        qty: it.qty ?? '0',
        unit_price: it.unit_price ?? '0',
        source_id: it.source_id,
        remark: it.remark,
      }))
    if (items.length === 0) {
      message.warning('先填至少一行明细再试算')
      return
    }

    setLoading(true)
    try {
      const res = await quotationApi.preview({ currency_code: values.currency_code, items })
      setPreview(res)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '试算失败')
    } finally {
      setLoading(false)
    }
  }

  return { preview, loading, run, reset }
}

/** 试算结果条。毛利为负时用错误色：这一行报出去就是亏。 */
export function QuotationPreviewAlert({ preview }: { preview: QuotationPreview | null }) {
  if (!preview) return null

  const loss = Number(preview.margin_rate) < 0
  const detail = preview.items
    .map(
      (it) =>
        `${it.sku_code} × ${it.qty}：报价 ${money(it.amount)} ${preview.currency_code}，成本 ${money(
          it.cost_amount,
        )}${it.source_name ? `（${it.source_name}）` : ''}`,
    )
    .join('\n')

  return (
    <Alert
      style={{ marginTop: 12 }}
      type={loss ? 'error' : 'info'}
      showIcon
      message={`试算：报价 ${money(preview.amount_total)} ${preview.currency_code} ｜ 折本币 ${money(
        preview.amount_base_total,
      )} ｜ 成本 ${money(preview.cost_total)} ｜ 毛利 ${money(
        preview.gross_profit,
      )} ｜ 毛利率 ${formatPercent(preview.margin_rate)}`}
      description={<pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{detail}</pre>}
    />
  )
}
