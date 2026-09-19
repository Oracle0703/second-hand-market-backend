import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProCard } from '@ant-design/pro-components'
import { Alert, Button, Descriptions, Form, Input, message } from 'antd'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth-store'
import { passwordHelp, passwordPattern } from '@/utils/password'

type Values = { old_password: string; new_password: string; confirm_password: string }

export function SecurityPage() {
  const navigate = useNavigate()
  const [form] = Form.useForm<Values>()
  const saving = useRef(false)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState('')
  const accountQuery = useQuery({
    queryKey: ['admin-account'],
    queryFn: async () => (await api.adminAccount()).data.data.account
  })

  const submit = async ({ old_password, new_password }: Values) => {
    if (saving.current) return
    saving.current = true
    setIsSaving(true)
    setError('')
    const ownerVersion = useAuthStore.getState().sessionVersion
    try {
      await api.adminChangePassword({ old_password, new_password })
      if (useAuthStore.getState().sessionVersion !== ownerVersion) return
      form.resetFields()
      message.success('密码已修改，请重新登录')
      useAuthStore.getState().clear()
      navigate('/admin/login', { replace: true })
    } catch (err) {
      if (useAuthStore.getState().sessionVersion === ownerVersion) {
        setError(err instanceof Error ? err.message : '密码修改失败，请稍后重试')
      }
    } finally {
      saving.current = false
      setIsSaving(false)
    }
  }

  const account = accountQuery.data
  return (
    <PageContainer title="安全设置">
      <ProCard title="当前管理员" style={{ marginBottom: 16 }} loading={accountQuery.isLoading}>
        {accountQuery.error ? <Alert type="error" showIcon message="账号信息加载失败" action={<Button onClick={() => void accountQuery.refetch()}>重试</Button>} /> : null}
        {account ? <Descriptions column={{ xs: 1, sm: 2 }} items={[
          { key: 'username', label: '账号', children: account.username },
          { key: 'role', label: '角色', children: account.role === 'SUPER_ADMIN' ? '超级管理员' : '管理员' }
        ]} /> : null}
      </ProCard>
      <ProCard title="修改登录密码">
        <div style={{ maxWidth: 520 }}>
          <Alert type="info" showIcon message="修改后，当前账号在所有设备上都需要使用新密码登录。" style={{ marginBottom: 24 }} />
          {error ? <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} /> : null}
          <Form<Values> name="admin-security" form={form} layout="vertical" onFinish={submit} disabled={isSaving} requiredMark={false}>
            <Form.Item name="old_password" label="旧密码" rules={[{ required: true, message: '请输入旧密码' }]}>
              <Input.Password autoComplete="current-password" maxLength={72} />
            </Form.Item>
            <Form.Item name="new_password" label="新密码" dependencies={['old_password']} extra={passwordHelp} rules={[
              { required: true, message: '请输入新密码' },
              { pattern: passwordPattern, message: passwordHelp },
              ({ getFieldValue }) => ({ validator: async (_, value) => {
                if (value && value === getFieldValue('old_password')) throw new Error('新密码不能与旧密码相同')
                if (value?.toLowerCase() === 'admin@123456') throw new Error('不能使用公开的初始密码')
              } })
            ]}>
              <Input.Password autoComplete="new-password" maxLength={72} />
            </Form.Item>
            <Form.Item name="confirm_password" label="确认新密码" dependencies={['new_password']} rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({ validator: async (_, value) => {
                if (value && value !== getFieldValue('new_password')) throw new Error('两次输入的新密码不一致')
              } })
            ]}>
              <Input.Password autoComplete="new-password" maxLength={72} />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={isSaving} disabled={isSaving}>修改密码并重新登录</Button>
          </Form>
        </div>
      </ProCard>
    </PageContainer>
  )
}
