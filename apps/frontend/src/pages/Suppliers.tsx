import { useState } from 'react'

import { supplierApi } from '@/api/supplier'
import type { SupplierInput } from '@/api/supplier'
import { Field } from '@/components/Field'
import { Modal } from '@/components/Modal'
import { Pagination } from '@/components/Pagination'
import { StatusBadge } from '@/components/StatusBadge'
import { PAGE_SIZE, usePagedResource } from '@/hooks/usePagedResource'
import type { Supplier } from '@/types'

const emptyForm: SupplierInput = {
  code: '',
  name: '',
  contact: '',
  phone: '',
  email: '',
  address: '',
  status: 1,
  remark: '',
}

export function Suppliers() {
  const list = usePagedResource<Supplier>((p) => supplierApi.list(p))

  const [search, setSearch] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Supplier | null>(null)
  const [form, setForm] = useState<SupplierInput>(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  function setField<K extends keyof SupplierInput>(key: K, value: SupplierInput[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormError('')
    setModalOpen(true)
  }

  const openEdit = (item: Supplier) => {
    setEditing(item)
    setForm({
      code: item.code,
      name: item.name,
      contact: item.contact,
      phone: item.phone,
      email: item.email,
      address: item.address,
      status: item.status,
      remark: item.remark,
    })
    setFormError('')
    setModalOpen(true)
  }

  const save = async () => {
    if (!form.code.trim() || !form.name.trim()) {
      setFormError('编码与名称为必填项')
      return
    }
    setSaving(true)
    setFormError('')
    try {
      if (editing) {
        await supplierApi.update(editing.id, form)
      } else {
        await supplierApi.create(form)
      }
      setModalOpen(false)
      list.reload()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (item: Supplier) => {
    if (!window.confirm(`确认删除供应商 ${item.name}？`)) return
    try {
      await supplierApi.remove(item.id)
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
          placeholder="搜索编码 / 名称 / 联系人"
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
          新建供应商
        </button>
      </div>

      {list.error && <div className="form-error">{list.error}</div>}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>编码</th>
              <th>名称</th>
              <th>联系人</th>
              <th>电话</th>
              <th>地址</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.items.map((item) => (
              <tr key={item.id}>
                <td className="cell-mono">{item.code}</td>
                <td className="cell-strong">{item.name}</td>
                <td>{item.contact || '—'}</td>
                <td>{item.phone || '—'}</td>
                <td>{item.address || '—'}</td>
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
          <div className="table-empty">暂无供应商数据</div>
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
        title={editing ? `编辑供应商 · ${editing.code}` : '新建供应商'}
        onClose={() => setModalOpen(false)}
        footer={
          <>
            <button type="button" className="btn" onClick={() => setModalOpen(false)}>
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
          <Field label="供应商编码" required>
            <input
              className="input"
              value={form.code}
              placeholder="如 SUP001"
              onChange={(e) => setField('code', e.target.value)}
            />
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

          <Field label="名称" required>
            <input
              className="input"
              value={form.name}
              onChange={(e) => setField('name', e.target.value)}
            />
          </Field>

          <Field label="联系人">
            <input
              className="input"
              value={form.contact}
              onChange={(e) => setField('contact', e.target.value)}
            />
          </Field>

          <Field label="电话">
            <input
              className="input"
              value={form.phone}
              onChange={(e) => setField('phone', e.target.value)}
            />
          </Field>

          <Field label="邮箱">
            <input
              className="input"
              value={form.email}
              onChange={(e) => setField('email', e.target.value)}
            />
          </Field>

          <Field label="地址" className="full">
            <input
              className="input"
              value={form.address}
              onChange={(e) => setField('address', e.target.value)}
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
