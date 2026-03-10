import { FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../services/api'
import { useAuthStore } from '../stores/auth-store'
import type { LoginType } from '../types/auth'

export function LoginPage() {
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)
  const [loginType, setLoginType] = useState<LoginType>('MERCHANT')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      const res = await api.login({ login_type: loginType, username, password })
      const data = res.data.data
      setAuth({
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        tokenScope: data.token_scope ?? 'full',
        user: data.user
      })
      if (loginType === 'ADMIN') {
        navigate('/admin/merchants/reviews')
      } else if (data.token_scope === 'onboarding') {
        navigate('/register/status')
      } else {
        navigate('/merchant/dashboard')
      }
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <main className="auth-page">
      <form className="card" onSubmit={onSubmit}>
        <h1>商家后台登录</h1>
        <label>
          登录类型
          <select value={loginType} onChange={(e) => setLoginType(e.target.value as LoginType)}>
            <option value="MERCHANT">商家</option>
            <option value="ADMIN">管理员</option>
          </select>
        </label>
        <label>
          账号
          <input value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <label>
          密码
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {error ? <p className="error">{error}</p> : null}
        <button type="submit">登录</button>
      </form>
    </main>
  )
}
