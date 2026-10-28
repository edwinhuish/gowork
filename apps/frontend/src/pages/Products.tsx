import { useEffect, useState } from 'react'

import { productApi } from '@/api/product'
import type { ProductInput } from '@/api/product'
import { supplierApi } from '@/api/supplier'
import { Field } from '@/components/Field'
import { Modal } from '@/components/Modal'
import { Pagination } from '@/components/Pagination'
import { StatusBadge } from '@/components/StatusBadge'
import { useFormDraft } from '@/hooks/useFormDraft'
import { PAGE_SIZE, usePagedResource } from '@/hooks/usePagedResource'
import type { Product, Supplier } from '@/types'

const emptyForm: ProductInput = {
  sku_code: '',
  name_cn: '',
  name_en: '',
  category: '',
  spec: '',
  unit: 'PCS',
  supplier_id: null,
  cost_price: 0,
  sale_price: 0,
  status: 1,
  remark: '',
}

// 与后端 model.SkuCode 的大类约定保持一致。
const categories = [
  { code: 'HT', label: '手动工具' },
  { code: 'PT', label: '电动工具' },
  { code: 'FS', label: '紧固件' },
  { code: 'BH', label: '建筑五金' },
  { code: 'LK', label: '锁具' },
  { code: 'PL', label: '水暖管件' },
  { code: 'MS', label: '测量工具' },
  { code: 'AB', label: '磨料耗材' },
  { code: 'EL', label: '电气' },
  { code: 'PK', label: '包装辅料' },
]

export function Products() {
  const list = usePagedResource<Product>((p) => productApi.list(p))

  const [suppliers, setSuppliers] = useState<Supplier[]>([])
  const [search, setSearch] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Product | null>(null)
  const [form, setForm] = useState<ProductInput>(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  // 新建态的内容只活在这个组件里，被登录态打断就彻底没了，所以单独存一份草稿。
  const draft = useFormDraft('product:create', form, modalOpen && !editing)

  useEffect(() => {
    supplierApi
      .list({ page_size: 200 })
      .then((res) => setSuppliers(res.list))
      .catch(() => setSuppliers([]))
  }, [])

  function setField<K extends keyof ProductInput>(key: K, value: ProductInput[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const openCreate = () => {
    setEditing(null)
    setForm(draft.take() ?? emptyForm)
    setFormError('')
    setModalOpen(true)
  }

  const openEdit = (item: Product) => {
    setEditing(item)
    setForm({
      sku_code: item.sku_code,
      name_cn: item.name_cn,
      name_en: item.name_en,
      category: item.category,
      spec: item.spec,
      unit: item.unit,
      supplier_id: item.supplier_id,
      cost_price: item.cost_price,
      sale_price: item.sale_price,
      status: item.status,
      remark: item.remark,
    })
    setFormError('')
    setModalOpen(true)
  }

  // 取消或关闭视为主动放弃，草稿一并清掉。
  // 被登录态打断时不会走到这里（组件直接卸载），草稿因此得以保留到下次新建。
  const close = () => {
    draft.clear()
    setModalOpen(false)
  }

  const save = async () => {
    if (!form.sku_code.trim() || !form.name_cn.trim()) {
      setFormError('SKU 编码与中文品名为必填项')
      return
    }
    setSaving(true)
    setFormError('')
    try {
      if (editing) {
        await productApi.update(editing.id, form)
      } else {
        await productApi.create(form)
      }
      draft.clear()
      setModalOpen(false)
      list.reload()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (item: Product) => {
    if (!window.confirm(`确认删除产品 ${item.sku_code}？`)) return
    try {
      await productApi.remove(item.id)
      list.reload()
    } catch (err) {
      window.alert(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <>
      <div className="toolbar">
        <input
          className="input input-search"
          placeholder="搜索 SKU / 品名"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') list.setKeyword(search.trim())
          }}
        />
        <button type="button" className="btn" onClick={() => list.setKeyword(search.trim())}>
          搜索
        </button>
        <div className="toolbar-spacer" />
        <button type="button" className="btn btn-primary" onClick={openCreate}>
          新建产品
        </button>
      </div>

      {list.error && <div className="form-error">{list.error}</div>}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>SKU 编码</th>
              <th>品名</th>
              <th>类别</th>
              <th>规格</th>
              <th>单位</th>
              <th>供应商</th>
              <th>成本价</th>
              <th>售价</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.items.map((item) => (
              <tr key={item.id}>
                <td className="cell-mono">{item.sku_code}</td>
                <td>
                  <div className="cell-strong">{item.name_cn}</div>
                  {item.name_en && (
                    <div style={{ color: 'var(--muted)', fontSize: 12.5 }}>{item.name_en}</div>
                  )}
                </td>
                <td>
                  <span className="badge badge-cat">{item.category || '—'}</span>
                </td>
                <td>{item.spec || '—'}</td>
                <td>{item.unit}</td>
                <td>{item.supplier?.name ?? '—'}</td>
                <td>¥{item.cost_price.toFixed(2)}</td>
                <td>¥{item.sale_price.toFixed(2)}</td>
                <td>
                  <StatusBadge status={item.status} />
                </td>
                <td>
                  <div className="cell-actions">
                    <button type="button" className="link-btn" onClick={() => openEdit(item)}>
                      编辑
                    </button>
                    <button type="button" className="link-btn danger" onClick={() => remove(item)}>
                      删除
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!list.loading && list.items.length === 0 && (
          <div className="table-empty">暂无产品数据</div>
        )}
        {list.loading && <div className="table-empty">加载中…</div>}
      </div>

      <Pagination
        page={list.page}
        pageSize={PAGE_SIZE}
        total={list.total}
        onChange={list.setPage}
      />

      <Modal
        open={modalOpen}
        title={editing ? `编辑产品 · ${editing.sku_code}` : '新建产品'}
        onClose={close}
        footer={
          <>
            <button type="button" className="btn" onClick={close}>
              取消
            </button>
            <button type="button" className="btn btn-primary" onClick={save} disabled={saving}>
              {saving ? '保存中…' : '保存'}
            </button>
          </>
        }
      >
        {formError && <div className="form-error">{formError}</div>}

        <div className="form-grid">
          <Field label="SKU 编码" required>
            <input
              className="input"
              value={form.sku_code}
              placeholder="如 BH01-0023-SS"
              onChange={(e) => setField('sku_code', e.target.value)}
            />
          </Field>

          <Field label="类别">
            <select
              className="select"
              value={form.category}
              onChange={(e) => setField('category', e.target.value)}
            >
              <option value="">未分类</option>
              {categories.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.code} · {c.label}
                </option>
              ))}
            </select>
          </Field>

          <Field label="中文品名" required>
            <input
              className="input"
              value={form.name_cn}
              onChange={(e) => setField('name_cn', e.target.value)}
            />
          </Field>

          <Field label="英文品名">
            <input
              className="input"
              value={form.name_en}
              onChange={(e) => setField('name_en', e.target.value)}
            />
          </Field>

          <Field label="规格">
            <input
              className="input"
              value={form.spec}
              placeholder="如 100×75×3 mm"
              onChange={(e) => setField('spec', e.target.value)}
            />
          </Field>

          <Field label="单位">
            <input
              className="input"
              value={form.unit}
              onChange={(e) => setField('unit', e.target.value)}
            />
          </Field>

          <Field label="供应商">
            <select
              className="select"
              value={form.supplier_id ?? ''}
              onChange={(e) => setField('supplier_id', e.target.value ? Number(e.target.value) : null)}
            >
              <option value="">未指定</option>
              {suppliers.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>

          <Field label="状态">
            <select
              className="select"
              value={form.status ?? 1}
              onChange={(e) => setField('status', Number(e.target.value))}
            >
              <option value={1}>启用</option>
              <option value={0}>停用</option>
            </select>
          </Field>

          <Field label="成本价">
            <input
              className="input"
              type="number"
              step="0.01"
              min="0"
              value={form.cost_price}
              onChange={(e) => setField('cost_price', Number(e.target.value) || 0)}
            />
          </Field>

          <Field label="售价">
            <input
              className="input"
              type="number"
              step="0.01"
              min="0"
              value={form.sale_price}
              onChange={(e) => setField('sale_price', Number(e.target.value) || 0)}
            />
          </Field>

          <Field label="备注" className="full">
            <input
              className="input"
              value={form.remark}
              onChange={(e) => setField('remark', e.target.value)}
            />
          </Field>
        </div>
      </Modal>
    </>
  )
}
