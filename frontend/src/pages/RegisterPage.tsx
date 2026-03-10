import { FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../services/api'

export function RegisterPage() {
  const navigate = useNavigate()
  const [form, setForm] = useState({
    merchant_name: '',
    contact_name: '',
    phone: '',
    username: '',
    password: ''
  })
  const [error, setError] = useState('')

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      await api.register({ ...form, license_file_id: 1 })
      navigate('/login')
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <main className="auth-page">
      <form className="card" onSubmit={onSubmit}>
        <h1>商家注册</h1>
        {Object.entries(form).map(([key, value]) => (
          <label key={key}>
            {key}
            <input value={value} onChange={(e) => setForm((prev) => ({ ...prev, [key]: e.target.value }))} />
          </label>
        ))}
        {error ? <p className="error">{error}</p> : null}
        <button type="submit">提交注册</button>
      </form>
    </main>
  )
}
