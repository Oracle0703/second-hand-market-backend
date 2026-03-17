import { LoginFormPage, ProFormText } from '@ant-design/pro-components'
import { message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth-store'

type LoginFormValues = {
  username: string
  password: string
}

export function AdminLoginPage() {
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)

  const onFinish = async (values: LoginFormValues) => {
    try {
      const res = await api.login({
        login_type: 'ADMIN',
        username: values.username,
        password: values.password
      })
      const data = res.data.data
      setAuth({
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        tokenScope: data.token_scope ?? 'full',
        user: data.user
      })
      navigate('/admin/merchants/reviews')
      return true
    } catch (err) {
      message.error((err as Error).message)
      return false
    }
  }

  return (
    <LoginFormPage<LoginFormValues>
      title="系统管理入口"
      subTitle="管理员登录"
      onFinish={onFinish}
      submitter={{
        searchConfig: {
          submitText: '管理员登录'
        }
      }}
      containerStyle={{ backgroundColor: '#f5f7fa' }}
    >
      <ProFormText
        name="username"
        label="账号"
        rules={[{ required: true, message: '请输入账号' }]}
        fieldProps={{ autoComplete: 'username' }}
      />
      <ProFormText.Password
        name="password"
        label="密码"
        rules={[{ required: true, message: '请输入密码' }]}
        fieldProps={{ autoComplete: 'current-password' }}
      />
    </LoginFormPage>
  )
}
