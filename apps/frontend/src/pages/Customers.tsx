import { useState } from 'react'

import { customerApi } from '@/api/customer'
import type { CustomerInput } from '@/api/customer'
import { Field } from '@/components/Field'
import { Modal } from '@/components/Modal'
import { Pagination } from '@/components/Pagination'
import { StatusBadge } from '@/components/StatusBadge'
import { useFormDraft } from '@/hooks/useFormDraft'
import { PAGE_SIZE, usePagedResource } from '@/hooks/usePagedResource'
import type { Customer } from '@/types'

const emptyForm: CustomerInput = {
  code: '',
  name: '',
  country: '',
  contact: '',
  phone: '',
  email: '',
  address: '',
  status: 1,
  remark: '',
}

export function Customers() {
  const list = usePagedResource<Customer>((p) => customerApi.list(p))

  const [search, setSearch] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Customer | null>(null)
  const [form, setForm] = useState<CustomerInput>(emptyForm)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  // 新建态的内容只活在这个组件里，被登录态打断就彻底没了，所以单独存一份草稿。
  const draft = useFormDraft('customer:create', form, modalOpen && !editing)

  function setField<K extends keyof CustomerInput>(key: K, value: CustomerInput[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const openCreate = () => {
    setEditing(null)
    setForm(draft.take() ?? emptyForm)
    setFormError('')
    setModalOpen(true)
  }

  const openEdit = (item: Customer) => {
    setEditing(item)
    setForm({
      code: item.code,
      name: item.name,
      country: item.country,
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

  // 取消或关闭视为主动放弃，草稿一并清掉。
  // 被登录态打断时不会走到这里（组件直接卸载），草稿因此得以保留到下次新建。
  const close = () => {
    draft.clear()
    setModalOpen(false)
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
        await customerApi.update(editing.id, form)
      } else {
        await customerApi.create(form)
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

  const remove = async (item: Customer) => {
    if (!window.confirm(`确认删除客户 ${item.name}？`)) return
    try {
      await customerApi.remove(item.id)
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
          新建客户
        </button>
      </div>

      {list.error && <div className="form-error">{list.error}</div>}

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>编码</th>
              <th>名称</th>
              <th>国家</th>
              <th>联系人</th>
              <th>电话</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {list.items.map((item) => (
              <tr key={item.id}>
                <td className="cell-mono">{item.code}</td>
                <td className="cell-strong">{item.name}</td>
                <td>{item.country || '—'}</td>
                <td>{item.contact || '—'}</td>
                <td>{item.phone || '—'}</td>
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
        {!list.loading && list.items.length === 0 && <div className="table-empty">暂无客户数据</div>}
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
        title={editing ? `编辑客户 · ${editing.code}` : '新建客户'}
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
          <Field label="客户编码" required>
            <input
              className="input"
              value={form.code}
              placeholder="如 CUS001"
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

          <Field label="国家">
            <input
              className="input"
              value={form.country}
              onChange={(e) => setField('country', e.target.value)}
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

          <Field label="地址">
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
