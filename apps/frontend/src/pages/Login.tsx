import { useState } from 'react'
import type { FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

import { useAuth } from '@/store/auth'

export function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  // 从路由守卫带过来的来源地址：登录后回到被打断的那一页。
  const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname || '/'

  // 预填演示账号，方便本地首次启动直接进入；生产环境请改掉默认密码。
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('admin123')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      await login(username.trim(), password)
      navigate(from, { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={onSubmit}>
        <h1 className="login-title">GoWork ERP</h1>
        <p className="login-sub">外贸采购与出口管理平台</p>

        {error && <div className="form-error">{error}</div>}

        <div className="login-field">
          <label className="form-label">用户名</label>
          <input
            className="input"
            value={username}
            autoComplete="username"
            onChange={(e) => setUsername(e.target.value)}
            required
          />
        </div>

        <div className="login-field">
          <label className="form-label">密码</label>
          <input
            className="input"
            type="password"
            value={password}
            autoComplete="current-password"
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </div>

        <button type="submit" className="btn btn-primary login-submit" disabled={loading}>
          {loading ? '登录中…' : '登录'}
        </button>

        <div className="login-hint">首次启动默认账号 admin / admin123</div>
      </form>
    </div>
  )
}
