import { useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer, ProCard, ProDescriptions, ProForm, ProFormText, type ProFormInstance } from '@ant-design/pro-components'
import { Alert, message } from 'antd'
import { getStatusText, ACCOUNT_STATUS_META } from '../constants/status'
import { api } from '../services/api'

type MerchantAccountInfo = {
  id: number
  username: string
  role: string
  status: string
  last_login_at?: string | null
}

type MerchantSecurityInfo = {
  password_updated_at?: string | null
  mfa_enabled: boolean
}

type MerchantAccountResp = {
  account: MerchantAccountInfo
  security: MerchantSecurityInfo
}

type PasswordFormValues = {
  old_password: string
  new_password: string
}

export function MerchantAccountPage() {
  const queryClient = useQueryClient()
  const formRef = useRef<ProFormInstance<PasswordFormValues>>()

  const accountQuery = useQuery({
    queryKey: ['merchant-account'],
    queryFn: async () => (await api.merchantAccount()).data.data as MerchantAccountResp
  })

  const passwordMutation = useMutation({
    mutationFn: async (values: PasswordFormValues) => api.merchantChangePassword(values),
    onSuccess: async () => {
      message.success('密码修改成功')
      formRef.current?.resetFields()
      await queryClient.invalidateQueries({ queryKey: ['merchant-account'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const onFinish = async (values: PasswordFormValues) => {
    await passwordMutation.mutateAsync(values)
    return true
  }

  if (accountQuery.isLoading) return <p>加载中...</p>
  if (accountQuery.error) return <p className="error">{(accountQuery.error as Error).message}</p>
  if (!accountQuery.data) return <p>暂无数据</p>

  const account = accountQuery.data.account
  const security = accountQuery.data.security

  return (
    <PageContainer title="账号设置">
      <ProCard title="账号信息" style={{ marginBottom: 16 }}>
        <ProDescriptions<MerchantAccountInfo>
          column={2}
          dataSource={account}
          columns={[
            { title: '账号ID', dataIndex: 'id' },
            { title: '用户名', dataIndex: 'username' },
            { title: '角色', dataIndex: 'role' },
            { title: '状态', key: 'status', render: (_, row) => getStatusText(ACCOUNT_STATUS_META, row.status) },
            { title: '最近登录', dataIndex: 'last_login_at', valueType: 'dateTime' }
          ]}
        />
      </ProCard>

      <ProCard title="安全信息" style={{ marginBottom: 16 }}>
        <ProDescriptions<MerchantSecurityInfo>
          column={2}
          dataSource={security}
          columns={[
            { title: '密码更新时间', dataIndex: 'password_updated_at', valueType: 'dateTime' },
            { title: 'MFA', key: 'mfa_enabled', render: (_, row) => (row.mfa_enabled ? '已开启' : '未开启') }
          ]}
        />
      </ProCard>

      <ProCard title="修改密码">
        {passwordMutation.error ? <Alert type="error" showIcon style={{ marginBottom: 16 }} message={(passwordMutation.error as Error).message} /> : null}
        <ProForm<PasswordFormValues>
          formRef={formRef}
          layout="vertical"
          onFinish={onFinish}
          submitter={{
            searchConfig: {
              submitText: '保存密码'
            },
            submitButtonProps: {
              loading: passwordMutation.isPending
            }
          }}
        >
          <ProFormText.Password name="old_password" label="旧密码" rules={[{ required: true, message: '请输入旧密码' }, { min: 8, message: '至少 8 位' }]} />
          <ProFormText.Password name="new_password" label="新密码" rules={[{ required: true, message: '请输入新密码' }, { min: 8, message: '至少 8 位' }]} />
        </ProForm>
      </ProCard>
    </PageContainer>
  )
}
