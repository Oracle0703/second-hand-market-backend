import { LoginFormPage, ProFormSelect, ProFormText } from '@ant-design/pro-components'
import { Button, Space, message } from 'antd'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth-store'
import type { LoginType } from '@/types/auth'

type LoginFormValues = {
  login_type: LoginType
  username: string
  password: string
}

export function LoginPage() {
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)

  const onFinish = async (values: LoginFormValues) => {
    try {
      const res = await api.login(values)
      const data = res.data.data
      setAuth({
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        tokenScope: data.token_scope ?? 'full',
        user: data.user
      })

      if (values.login_type === 'ADMIN') {
        navigate('/admin/merchants/reviews')
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
      initialValues={{ login_type: 'MERCHANT', username: 'yaner', password: '12345678' }}
      submitter={{
        searchConfig: {
          submitText: '登录'
        }
      }}
      actions={
        <Space size={8}>
          <span>还没有商家账号？</span>
          <Link to="/register">
            <Button type="link" style={{ paddingInline: 0 }}>
              去注册
            </Button>
          </Link>
        </Space>
      }
      containerStyle={{ backgroundColor: '#f5f7fa' }}
    >
      <ProFormSelect
        name="login_type"
        label="登录类型"
        rules={[{ required: true, message: '请选择登录类型' }]}
        options={[
          { label: '商家', value: 'MERCHANT' },
          { label: '管理员', value: 'ADMIN' }
        ]}
      />
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
