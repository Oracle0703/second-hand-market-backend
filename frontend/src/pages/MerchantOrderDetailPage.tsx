import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { api } from '../services/api'

export function MerchantOrderDetailPage() {
  const { orderId = '' } = useParams()
  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['order-detail', orderId],
    queryFn: async () => (await api.orderDetail(orderId)).data.data as any
  })

  const completeMutation = useMutation({
    mutationFn: async () => api.orderComplete(orderId, '完成交易'),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['order-detail', orderId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    }
  })

  const closeMutation = useMutation({
    mutationFn: async () => api.orderClose(orderId, '关闭订单'),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['order-detail', orderId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    }
  })

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>

  const data = detail.data
  const order = data.order_detail
  return (
    <section className="card">
      <h1>订单详情</h1>
      <p>订单号: {order.order_no}</p>
      <p>状态: {order.status}</p>
      <p>成交价: {order.deal_price_cent}</p>
      <p>
        商品: <Link to={`/merchant/products/${order.product.id}`}>{order.product.title}</Link> ({order.product.status})
      </p>

      {order.status === 'CREATED' && (
        <div className="toolbar">
          <button onClick={() => completeMutation.mutate()}>完成订单</button>
          <button onClick={() => closeMutation.mutate()}>关闭订单</button>
        </div>
      )}

      <h2>事件</h2>
      <ul>
        {(data.events ?? []).map((event: any) => (
          <li key={event.ID ?? event.id}>
            {event.EventType ?? event.event_type}: {(event.FromStatus ?? event.from_status) || '-'} -&gt; {event.ToStatus ?? event.to_status}
          </li>
        ))}
      </ul>

      {(completeMutation.error || closeMutation.error) ? <p className="error">{((completeMutation.error ?? closeMutation.error) as Error).message}</p> : null}
    </section>
  )
}
