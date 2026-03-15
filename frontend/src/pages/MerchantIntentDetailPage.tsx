import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer, ProDescriptions } from '@ant-design/pro-components'
import { Alert, Button, Tag, message } from 'antd'
import { useParams } from 'react-router-dom'
import { getStatusColor, getStatusText, INTENT_STATUS_META, PRODUCT_STATUS_META, type IntentStatus } from '../constants/status'
import { api } from '../services/api'

type IntentDetail = {
  id: number
  intent_no: string
  status: IntentStatus
  buyer_status_text: string
  product?: {
    id: number
    title: string
    status: string
  }
  contact_name?: string | null
  contact_phone?: string | null
  contact_wechat?: string | null
  message?: string | null
  merchant_note?: string | null
  close_reason?: string | null
  created_at: string
  updated_at: string
}

type IntentDetailResp = {
  intent: IntentDetail
}

export function MerchantIntentDetailPage() {
  const { intentId = '' } = useParams()
  const queryClient = useQueryClient()

  const detail = useQuery({
    queryKey: ['merchant-intent-detail', intentId],
    queryFn: async () => (await api.merchantIntentDetail(intentId)).data.data as IntentDetailResp
  })

  const contactedMutation = useMutation({
    mutationFn: async () => api.merchantIntentContacted(intentId),
    onSuccess: async () => {
      message.success('已标记为已联系')
      await queryClient.invalidateQueries({ queryKey: ['merchant-intent-detail', intentId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-intents'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const closeMutation = useMutation({
    mutationFn: async () => api.merchantIntentClose(intentId, { reason: 'NOT_INTERESTED' }),
    onSuccess: async () => {
      message.success('线索已关闭')
      await queryClient.invalidateQueries({ queryKey: ['merchant-intent-detail', intentId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-intents'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>
  if (!detail.data) return <p>暂无数据</p>

  const intent = detail.data.intent
  return (
    <PageContainer
      title="意向详情"
      subTitle={intent.intent_no}
      extra={
        intent.status === 'NEW'
          ? [
              <Button key="contacted" type="primary" loading={contactedMutation.isPending} onClick={() => contactedMutation.mutate()}>
                标记已联系
              </Button>,
              <Button key="close" danger loading={closeMutation.isPending} onClick={() => closeMutation.mutate()}>
                关闭线索
              </Button>
            ]
          : intent.status === 'CONTACTED'
            ? [
                <Button key="close" danger loading={closeMutation.isPending} onClick={() => closeMutation.mutate()}>
                  关闭线索
                </Button>
              ]
            : undefined
      }
    >
      {(contactedMutation.error || closeMutation.error) ? <Alert type="error" showIcon message={((contactedMutation.error ?? closeMutation.error) as Error).message} style={{ marginBottom: 16 }} /> : null}

      <ProDescriptions<IntentDetail>
        column={2}
        dataSource={intent}
        columns={[
          {
            title: '状态',
            dataIndex: 'status',
            render: (_, row) => <Tag color={getStatusColor(INTENT_STATUS_META, row.status)}>{getStatusText(INTENT_STATUS_META, row.status)}</Tag>
          },
          { title: '买家可见状态', dataIndex: 'buyer_status_text' },
          { title: '商品', key: 'product', render: (_, row) => row.product?.title ?? '-' },
          { title: '商品状态', key: 'product_status', render: (_, row) => getStatusText(PRODUCT_STATUS_META, row.product?.status) },
          { title: '联系人', dataIndex: 'contact_name' },
          { title: '手机号', dataIndex: 'contact_phone' },
          { title: '微信号', dataIndex: 'contact_wechat' },
          { title: '关闭原因', dataIndex: 'close_reason' },
          { title: '留言', dataIndex: 'message', span: 2 },
          { title: '商家备注', dataIndex: 'merchant_note', span: 2 },
          { title: '创建时间', dataIndex: 'created_at', valueType: 'dateTime' },
          { title: '更新时间', dataIndex: 'updated_at', valueType: 'dateTime' }
        ]}
      />
    </PageContainer>
  )
}
