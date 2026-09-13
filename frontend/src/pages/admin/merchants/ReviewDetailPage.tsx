import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  PageContainer,
  ProCard,
  ProDescriptions,
  ProTable
} from '@ant-design/pro-components'
import { Button, Form, Popconfirm, message } from 'antd'
import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '@/services/api'
import { PasswordInput } from '@/components/PasswordInput'
import { passwordHelp, passwordPattern } from '@/utils/password'

type Detail = {
  merchant_detail: {
    merchant_name: string
    contact_name: string
    contact_phone: string
    created_at: string
  }
  account: { username: string; status: string } | null
  audit_logs: {
    id: number
    action: string
    operator_id: number
    created_at: string
  }[]
}
export function ReviewDetailPage() {
  const { merchantId = '' } = useParams()
  const queryClient = useQueryClient()
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<{ password: string }>()
  const detail = useQuery({
    queryKey: ['admin-merchant-detail', merchantId],
    queryFn: async () =>
      (await api.adminMerchantReviewDetail(merchantId)).data.data as Detail
  })
  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p>{(detail.error as Error).message}</p>
  if (!detail.data) return null
  const { merchant_detail: merchant, account, audit_logs: logs } = detail.data
  const reload = () =>
    queryClient.invalidateQueries({
      queryKey: ['admin-merchant-detail', merchantId]
    })
  return (
    <PageContainer title="商户详情" subTitle={merchant.merchant_name}>
      <ProDescriptions
        column={2}
        dataSource={merchant}
        columns={[
          { title: '商户名称', dataIndex: 'merchant_name' },
          { title: '联系人', dataIndex: 'contact_name' },
          { title: '联系电话', dataIndex: 'contact_phone' },
          { title: '创建时间', dataIndex: 'created_at', valueType: 'dateTime' }
        ]}
      />
      {account && (
        <ProCard
          title={`登录账号：${account.username}`}
          style={{ marginTop: 16 }}
        >
          <p>账号状态：{account.status === 'ACTIVE' ? '启用' : '禁用'}</p>
          <Popconfirm
            title={
              account.status === 'ACTIVE'
                ? '禁用后将立即退出所有登录会话，确认禁用？'
                : '确认启用账号？'
            }
            onConfirm={async () => {
              try {
                await api.adminSetMerchantStatus(
                  merchantId,
                  account.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE'
                )
                await reload()
                message.success('账号状态已更新')
              } catch (error) {
                message.error((error as Error).message)
              }
            }}
          >
            <Button danger={account.status === 'ACTIVE'}>
              {account.status === 'ACTIVE' ? '禁用账号' : '启用账号'}
            </Button>
          </Popconfirm>
          <Form
            form={form}
            layout="vertical"
            style={{ marginTop: 24 }}
            onFinish={async ({ password }) => {
              setSaving(true)
              try {
                await api.adminResetMerchantPassword(merchantId, password)
                form.resetFields()
                await reload()
                message.success('密码已重置，商户下次登录须修改密码')
              } catch (error) {
                message.error((error as Error).message)
              } finally {
                setSaving(false)
              }
            }}
          >
            <Form.Item
              name="password"
              label="重置初始密码"
              extra={`${passwordHelp}。重置前请复制保存，重置后旧会话立即失效。`}
              rules={[
                { required: true },
                { pattern: passwordPattern, message: passwordHelp }
              ]}
            >
              <PasswordInput />
            </Form.Item>
            <Button htmlType="submit" loading={saving}>
              重置密码
            </Button>
          </Form>
        </ProCard>
      )}
      <ProTable
        rowKey="id"
        headerTitle="操作记录"
        search={false}
        options={false}
        dataSource={logs}
        columns={[
          { title: '时间', dataIndex: 'created_at', valueType: 'dateTime' },
          { title: '操作', dataIndex: 'action' },
          { title: '管理员 ID', dataIndex: 'operator_id' }
        ]}
      />
    </PageContainer>
  )
}
