import { useRef } from 'react'
import { useMutation } from '@tanstack/react-query'
import { PageContainer, ProCard, ProForm, ProFormText, type ProFormInstance } from '@ant-design/pro-components'
import { Alert, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth-store'

type PasswordFormValues = {
  current_password: string
  new_password: string
  confirm_password: string
}

export function SecurityPage() {
  const formRef = useRef<ProFormInstance<PasswordFormValues>>()
  const navigate = useNavigate()
  const clear = useAuthStore((state) => state.clear)

  const passwordMutation = useMutation({
    mutationFn: async (values: PasswordFormValues) =>
      api.adminChangePassword({
        current_password: values.current_password,
        new_password: values.new_password
      }),
    onSuccess: () => {
      clear()
      message.success('密码修改成功，请重新登录')
      navigate('/admin/login', { replace: true })
    },
    onError: (error) => {
      message.error((error as Error).message)
    }
  })

  const onFinish = async (values: PasswordFormValues) => {
    await passwordMutation.mutateAsync(values)
    return true
  }

  return (
    <PageContainer title="安全设置">
      <ProCard title="修改密码">
        {passwordMutation.error ? (
          <Alert type="error" showIcon style={{ marginBottom: 16 }} message={(passwordMutation.error as Error).message} />
        ) : null}
        <ProForm<PasswordFormValues>
          formRef={formRef}
          layout="vertical"
          onFinish={onFinish}
          submitter={{
            searchConfig: { submitText: '保存密码' },
            submitButtonProps: { loading: passwordMutation.isPending }
          }}
        >
          <ProFormText.Password
            name="current_password"
            label="当前密码"
            fieldProps={{ autoComplete: 'current-password' }}
            rules={[{ required: true, message: '请输入当前密码' }]}
          />
          <ProFormText.Password
            name="new_password"
            label="新密码"
            fieldProps={{ autoComplete: 'new-password', maxLength: 72 }}
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 12, message: '新密码至少 12 位' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || value !== getFieldValue('current_password')) return Promise.resolve()
                  return Promise.reject(new Error('新密码不能与当前密码相同'))
                }
              })
            ]}
          />
          <ProFormText.Password
            name="confirm_password"
            label="确认新密码"
            dependencies={['new_password']}
            fieldProps={{ autoComplete: 'new-password', maxLength: 72 }}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || value === getFieldValue('new_password')) return Promise.resolve()
                  return Promise.reject(new Error('两次输入的新密码不一致'))
                }
              })
            ]}
          />
        </ProForm>
      </ProCard>
    </PageContainer>
  )
}
