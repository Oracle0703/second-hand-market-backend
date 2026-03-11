import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { api } from '../services/api'

export function MerchantIntentDetailPage() {
  const { intentId = '' } = useParams()
  const queryClient = useQueryClient()

  const detail = useQuery({
    queryKey: ['merchant-intent-detail', intentId],
    queryFn: async () => (await api.merchantIntentDetail(intentId)).data.data as any
  })

  const contactedMutation = useMutation({
    mutationFn: async () => api.merchantIntentContacted(intentId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['merchant-intent-detail', intentId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-intents'] })
    }
  })

  const closeMutation = useMutation({
    mutationFn: async () => api.merchantIntentClose(intentId, { reason: 'NOT_INTERESTED' }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['merchant-intent-detail', intentId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-intents'] })
    }
  })

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>

  const intent = detail.data.intent
  return (
    <section className="card">
      <h1>意向详情</h1>
      <p>意向单号: {intent.intent_no}</p>
      <p>状态: {intent.status}</p>
      <p>买家可见状态: {intent.buyer_status_text}</p>
      <p>商品: {intent.product?.title}</p>
      <p>联系人: {intent.contact_name || '-'}</p>
      <p>手机号: {intent.contact_phone || '-'}</p>
      <p>微信号: {intent.contact_wechat || '-'}</p>
      <p>留言: {intent.message || '-'}</p>

      {intent.status === 'NEW' && (
        <div className="toolbar">
          <button onClick={() => contactedMutation.mutate()} disabled={contactedMutation.isPending}>
            标记已联系
          </button>
          <button onClick={() => closeMutation.mutate()} disabled={closeMutation.isPending}>
            关闭线索
          </button>
        </div>
      )}

      {intent.status === 'CONTACTED' && (
        <div className="toolbar">
          <button onClick={() => closeMutation.mutate()} disabled={closeMutation.isPending}>
            关闭线索
          </button>
        </div>
      )}

      {(contactedMutation.error || closeMutation.error) ? (
        <p className="error">{((contactedMutation.error ?? closeMutation.error) as Error).message}</p>
      ) : null}
    </section>
  )
}
