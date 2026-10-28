import { useEffect, useState } from 'react'

import { dashboardApi } from '../api/dashboard'
import type { DashboardSummary } from '../types'

export function Dashboard() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    dashboardApi
      .summary()
      .then(setSummary)
      .catch((err: Error) => setError(err.message))
  }, [])

  return (
    <>
      {error && <div className="form-error">{error}</div>}

      <div className="stat-grid">
        <div className="stat-card">
          <div className="stat-label">产品总数</div>
          <div className="stat-value">{summary?.product_total ?? '—'}</div>
          <div className="stat-hint">在售 {summary?.active_products ?? '—'} 个</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">供应商</div>
          <div className="stat-value">{summary?.supplier_total ?? '—'}</div>
          <div className="stat-hint">1688 店铺 / 工厂</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">客户</div>
          <div className="stat-value">{summary?.customer_total ?? '—'}</div>
          <div className="stat-hint">进口商与终端门店</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">待发货订单</div>
          <div className="stat-value">0</div>
          <div className="stat-hint">订单模块待接入</div>
        </div>
      </div>

      <div className="card" style={{ marginTop: 18 }}>
        <div style={{ fontWeight: 600, marginBottom: 8 }}>骨架说明</div>
        <p style={{ color: 'var(--muted)', margin: 0, fontSize: 13.5, lineHeight: 1.75 }}>
          当前已完成基础主数据（产品 / 供应商 / 客户）的完整分层实现，可作为后续模块的模板：
          新增业务模块时，按 <code>model → repository → service → handler → router</code> 的顺序各加一层，
          并在 <code>internal/bootstrap/app.go</code> 里完成装配即可。订单、采购、库存、出货批次等模块留待扩展。
        </p>
      </div>
    </>
  )
}
