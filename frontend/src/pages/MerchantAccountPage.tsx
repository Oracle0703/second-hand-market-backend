import { FormEvent, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../services/api'

export function MerchantAccountPage() {
  const queryClient = useQueryClient()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')

  const accountQuery = useQuery({
    queryKey: ['merchant-account'],
    queryFn: async () => (await api.merchantAccount()).data.data as any
  })

  const passwordMutation = useMutation({
    mutationFn: async () => api.merchantChangePassword({ old_password: oldPassword, new_password: newPassword }),
    onSuccess: async () => {
      setOldPassword('')
      setNewPassword('')
      await queryClient.invalidateQueries({ queryKey: ['merchant-account'] })
    }
  })

  const submitPassword = (e: FormEvent) => {
    e.preventDefault()
    passwordMutation.mutate()
  }

  if (accountQuery.isLoading) return <p>加载中...</p>
  if (accountQuery.error) return <p className="error">{(accountQuery.error as Error).message}</p>

  const account = accountQuery.data.account
  const security = accountQuery.data.security

  return (
    <section className="card">
      <h1>账号设置</h1>
      <p>账号ID: {account.id}</p>
      <p>用户名: {account.username}</p>
      <p>角色: {account.role}</p>
      <p>状态: {account.status}</p>
      <p>最近登录: {account.last_login_at || '-'}</p>
      <p>密码更新时间: {security.password_updated_at || '-'}</p>

      <form className="card" onSubmit={submitPassword}>
        <h2>修改密码</h2>
        <label>
          旧密码
          <input type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} />
        </label>
        <label>
          新密码
          <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
        </label>
        {passwordMutation.error ? <p className="error">{(passwordMutation.error as Error).message}</p> : null}
        <button type="submit" disabled={passwordMutation.isPending || !oldPassword || !newPassword}>
          保存密码
        </button>
      </form>
    </section>
  )
}
