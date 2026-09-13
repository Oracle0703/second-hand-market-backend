import { LoginFormPage, ProFormText } from '@ant-design/pro-components'
import { message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth-store'

type LoginFormValues = {
  username: string
  password: string
}

export function LoginPage() {
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)

  const onFinish = async (values: LoginFormValues) => {
    try {
      const res = await api.login({
        login_type: 'MERCHANT',
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

      if (data.user.must_change_password) {
        navigate('/merchant/account')
      } else if (data.token_scope === 'onboarding') {
        navigate('/register/status')
      } else {
        navigate('/merchant/dashboard')
      }
      return true
    } catch (err) {
      message.error((err as Error).message)
      return false
    }
  }

  return (
    <LoginFormPage<LoginFormValues>
      title="广汉市瑞扬家具经营部"
      subTitle="商家后台管理系统"
      onFinish={onFinish}
      submitter={{
        searchConfig: {
          submitText: '登录'
        }
      }}
      actions={<span>账号由管理员分配，如需开通或重置密码请联系管理员。</span>}
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
