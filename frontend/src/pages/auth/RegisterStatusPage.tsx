import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getStatusText, MERCHANT_REVIEW_STATUS_META } from '@/constants/status'
import { api } from '@/services/api'

export function RegisterStatusPage() {
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({
    queryKey: ['merchant-profile'],
    queryFn: async () => (await api.merchantProfile()).data.data as any
  })
  const reapplyMutation = useMutation({
    mutationFn: async () => api.merchantReapply({}),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['merchant-profile'] })
    }
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  return (
    <section className="card">
      <h1>注册审核状态</h1>
      <p>商家：{data.merchant_info?.name}</p>
      <p>状态：{getStatusText(MERCHANT_REVIEW_STATUS_META, data.review_status)}</p>
      {data.reject_reason ? <p>驳回原因：{data.reject_reason}</p> : null}
      {data.review_status === 'REJECTED' ? (
        <button onClick={() => reapplyMutation.mutate()} disabled={reapplyMutation.isPending}>
          重新提交审核
        </button>
      ) : null}
      {reapplyMutation.error ? <p className="error">{(reapplyMutation.error as Error).message}</p> : null}
    </section>
  )
}
